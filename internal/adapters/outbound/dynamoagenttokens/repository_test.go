package dynamoagenttokens_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoagenttokens"
	"github.com/rtrydev/langler-backend/internal/domain/agenttoken"
)

func localClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	endpoint := os.Getenv("DYNAMODB_LOCAL_ENDPOINT")
	if endpoint == "" {
		t.Skip("DYNAMODB_LOCAL_ENDPOINT not set; start DynamoDB Local and set its endpoint")
	}
	return dynamodb.New(dynamodb.Options{
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("local", "local", ""),
		BaseEndpoint: aws.String(endpoint),
	})
}

func createTable(t *testing.T, client *dynamodb.Client) string {
	t.Helper()
	table := "agent-tokens-test-" + time.Now().UTC().Format("20060102150405.000000000")
	_, err := client.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName:   aws.String(table),
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("SK"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("PK"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("SK"), KeyType: types.KeyTypeRange},
		},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	t.Cleanup(func() {
		if _, err := client.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{TableName: aws.String(table)}); err != nil {
			t.Errorf("DeleteTable: %v", err)
		}
	})
	return table
}

func TestRepositoryLifecycleAndRateLimit(t *testing.T) {
	client := localClient(t)
	table := createTable(t, client)
	repo, err := dynamoagenttokens.NewRepository(client, table)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	ctx := context.Background()
	createdAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	token, err := agenttoken.New(
		"token-1",
		"user-1",
		"Claude Code",
		"abcd",
		[]agenttoken.Scope{agenttoken.ScopeReadReference},
		createdAt,
		createdAt.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := repo.Create(ctx, token, "hash-1"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Create(ctx, token, "hash-1"); !errors.Is(err, agenttoken.ErrAlreadyExists) {
		t.Fatalf("duplicate Create error = %v, want %v", err, agenttoken.ErrAlreadyExists)
	}

	listed, err := repo.List(ctx, "user-1")
	if err != nil || len(listed) != 1 || listed[0].ID != token.ID {
		t.Fatalf("List = %+v error = %v", listed, err)
	}
	found, err := repo.FindByHash(ctx, "hash-1")
	if err != nil || found.Owner != token.Owner {
		t.Fatalf("FindByHash = %+v error = %v", found, err)
	}

	usedAt := createdAt.Add(time.Hour)
	if err := repo.Touch(ctx, found, usedAt); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	listed, err = repo.List(ctx, "user-1")
	if err != nil || len(listed) != 1 || !listed[0].LastUsed.Equal(usedAt) {
		t.Fatalf("List after Touch = %+v error = %v", listed, err)
	}

	window := usedAt.Truncate(time.Minute)
	if err := repo.Consume(ctx, token.ID, window, 1); err != nil {
		t.Fatalf("first Consume: %v", err)
	}
	if err := repo.Consume(ctx, token.ID, window, 1); !errors.Is(err, agenttoken.ErrRateLimited) {
		t.Fatalf("second Consume error = %v, want %v", err, agenttoken.ErrRateLimited)
	}

	revokedAt := usedAt.Add(time.Minute)
	if err := repo.Revoke(ctx, token.Owner, token.ID, revokedAt); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	found, err = repo.FindByHash(ctx, "hash-1")
	if err != nil || !found.RevokedAt.Equal(revokedAt) || found.Active(revokedAt) {
		t.Fatalf("FindByHash after Revoke = %+v error = %v", found, err)
	}
	if err := repo.Revoke(ctx, "user-2", token.ID, revokedAt); !errors.Is(err, agenttoken.ErrNotFound) {
		t.Fatalf("cross-owner Revoke error = %v, want %v", err, agenttoken.ErrNotFound)
	}
}

func TestCreateSetsExpirySweepTTL(t *testing.T) {
	client := localClient(t)
	table := createTable(t, client)
	repo, err := dynamoagenttokens.NewRepository(client, table)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	ctx := context.Background()
	createdAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(24 * time.Hour)
	token, err := agenttoken.New("token-1", "user-1", "Claude Code", "abcd", []agenttoken.Scope{agenttoken.ScopeReadReference}, createdAt, expiresAt)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := repo.Create(ctx, token, "hash-1"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, key := range []map[string]types.AttributeValue{
		{"PK": &types.AttributeValueMemberS{Value: "USER#user-1"}, "SK": &types.AttributeValueMemberS{Value: "AGENTTOKEN#token-1"}},
		{"PK": &types.AttributeValueMemberS{Value: "AGENTTOKENHASH#hash-1"}, "SK": &types.AttributeValueMemberS{Value: "AGENTTOKEN"}},
	} {
		out, err := client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(table), Key: key})
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}
		ttl, ok := out.Item["expiresAtUnix"].(*types.AttributeValueMemberN)
		if !ok {
			t.Fatalf("expiresAtUnix missing or wrong type: %+v", out.Item["expiresAtUnix"])
		}
		wantTTL := expiresAt.Add(30 * 24 * time.Hour).Unix()
		if ttl.Value != strconv.FormatInt(wantTTL, 10) {
			t.Fatalf("expiresAtUnix = %s, want %d", ttl.Value, wantTTL)
		}
	}
}

func TestConsumeHoldsLimitUnderConcurrentLoad(t *testing.T) {
	client := localClient(t)
	table := createTable(t, client)
	repo, err := dynamoagenttokens.NewRepository(client, table)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	ctx := context.Background()
	window := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	const limit = agenttoken.RequestsPerMinute
	const attempts = limit * 3

	results := make(chan error, attempts)
	for range attempts {
		go func() { results <- repo.Consume(ctx, "token-under-load", window, limit) }()
	}
	var accepted, rejected int
	for range attempts {
		switch err := <-results; {
		case err == nil:
			accepted++
		case errors.Is(err, agenttoken.ErrRateLimited):
			rejected++
		default:
			t.Fatalf("Consume: unexpected error %v", err)
		}
	}
	if accepted != limit {
		t.Fatalf("accepted = %d, want %d (rejected %d)", accepted, limit, rejected)
	}
	if rejected != attempts-limit {
		t.Fatalf("rejected = %d, want %d", rejected, attempts-limit)
	}
}
