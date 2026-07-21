package dynamoprogress_test

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoprogress"
	domain "github.com/rtrydev/langler-backend/internal/domain/progress"
)

func localClient(t *testing.T) *dynamodb.Client {
	t.Helper()

	endpoint := os.Getenv("DYNAMODB_LOCAL_ENDPOINT")
	if endpoint == "" {
		t.Skip("DYNAMODB_LOCAL_ENDPOINT not set; start DynamoDB Local (docker run -p 8000:8000 amazon/dynamodb-local) and set DYNAMODB_LOCAL_ENDPOINT=http://localhost:8000")
	}
	return dynamodb.New(dynamodb.Options{
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("local", "local", ""),
		BaseEndpoint: aws.String(endpoint),
	})
}

func createTable(t *testing.T, client *dynamodb.Client) string {
	t.Helper()

	table := "progress-test-" + time.Now().UTC().Format("20060102150405.000000000")
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
		_, _ = client.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{TableName: aws.String(table)})
	})
	return table
}

func seedItems(t *testing.T, client *dynamodb.Client, table string, items []map[string]any) {
	t.Helper()

	for _, item := range items {
		marshalled, err := attributevalue.MarshalMap(item)
		if err != nil {
			t.Fatalf("MarshalMap: %v", err)
		}
		if _, err := client.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String(table),
			Item:      marshalled,
		}); err != nil {
			t.Fatalf("PutItem: %v", err)
		}
	}
}

func TestSnapshotClampsOverflowedDueDates(t *testing.T) {
	client := localClient(t)
	table := createTable(t, client)
	// A row written before the MaxIntervalDays cap: the five-digit-year dueDate
	// does not parse as RFC 3339, which used to fail the whole read.
	seedItems(t, client, table, []map[string]any{
		{
			"PK": "USER#user-1", "SK": "SRS#ja#vocab#N5#1000001", "itemId": "N5#1000001",
			"language": "ja", "kind": "vocab", "headword": "週末", "gloss": "weekend",
			"easeFactor": 2.5, "intervalDays": 36000, "repetitions": 40,
			"dueDate":   "36646-03-17T00:00:00Z",
			"createdAt": "2026-07-19T00:00:00Z", "updatedAt": "2026-07-20T10:00:00Z", "version": 40,
		},
	})

	repo, err := dynamoprogress.NewRepository(client, table)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	ctx := context.Background()

	snapshot, err := repo.Snapshot(ctx, "user-1")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(snapshot.Items))
	}
	item := snapshot.Items[0]
	if item.IntervalDays != domain.MaxIntervalDays {
		t.Errorf("IntervalDays = %d, want the %d cap", item.IntervalDays, domain.MaxIntervalDays)
	}
	if want := time.Date(2027, 7, 20, 10, 0, 0, 0, time.UTC); !item.DueDate.Equal(want) {
		t.Errorf("DueDate = %v, want updatedAt plus the cap (%v)", item.DueDate, want)
	}
	if item.Version != 40 {
		t.Errorf("Version = %d, want 40 so the next save still matches", item.Version)
	}

	items, err := repo.GetItems(ctx, "user-1", "ja", []string{"vocab#N5#1000001"})
	if err != nil {
		t.Fatalf("GetItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("GetItems = %d items, want 1", len(items))
	}
}

func TestCoveredItemIDs(t *testing.T) {
	client := localClient(t)
	table := createTable(t, client)
	seedItems(t, client, table, []map[string]any{
		{"PK": "USER#user-1", "SK": "SRS#ja#vocab#N5#1000001", "itemId": "N5#1000001", "language": "ja", "kind": "vocab"},
		{"PK": "USER#user-1", "SK": "SRS#ja#vocab#N5#1000002", "itemId": "N5#1000002", "language": "ja", "kind": "vocab"},
		{"PK": "USER#user-1", "SK": "SRS#ja#grammar#N5#particle-wa", "itemId": "N5#particle-wa", "language": "ja", "kind": "grammar"},
		{"PK": "USER#user-1", "SK": "SRS#pl#vocab#A1#100", "itemId": "A1#100", "language": "pl", "kind": "vocab"},
		{"PK": "USER#user-2", "SK": "SRS#ja#vocab#N5#1000003", "itemId": "N5#1000003", "language": "ja", "kind": "vocab"},
	})

	repo, err := dynamoprogress.NewRepository(client, table)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	ctx := context.Background()

	vocab, err := repo.CoveredItemIDs(ctx, "user-1", "ja", domain.KindVocab)
	if err != nil {
		t.Fatalf("CoveredItemIDs: %v", err)
	}
	slices.Sort(vocab)
	if want := []string{"N5#1000001", "N5#1000002"}; !slices.Equal(vocab, want) {
		t.Errorf("vocab = %v, want %v", vocab, want)
	}

	grammar, err := repo.CoveredItemIDs(ctx, "user-1", "ja", domain.KindGrammar)
	if err != nil {
		t.Fatalf("CoveredItemIDs: %v", err)
	}
	if want := []string{"N5#particle-wa"}; !slices.Equal(grammar, want) {
		t.Errorf("grammar = %v, want %v", grammar, want)
	}

	none, err := repo.CoveredItemIDs(ctx, "user-3", "ja", domain.KindVocab)
	if err != nil {
		t.Fatalf("CoveredItemIDs: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("ids = %v, want none", none)
	}
}
