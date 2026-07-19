package dynamoassessments_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoassessments"
	domain "github.com/rtrydev/langler-backend/internal/domain/assessment"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
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

	table := "assessments-test-" + time.Now().UTC().Format("20060102150405.000000000")
	ctx := context.Background()
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
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

func sampleSession(id string, version int) domain.Session {
	started := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	return domain.Session{
		ID:       id,
		Language: "ja",
		Status:   domain.StatusInProgress,
		Bands:    []string{"N5", "N4", "N3", "N2", "N1"},
		Stages: []domain.Stage{{
			Band: "N5",
			Items: []domain.Item{{
				Kind:         domain.KindVocab,
				Prompt:       "犬",
				Options:      []string{"dog", "cat", "bird", "fish"},
				CorrectIndex: 0,
				ReferenceID:  "N5#1",
			}},
		}},
		StartedAt: started,
		Version:   version,
	}
}

func TestRepositoryRoundTripsSessions(t *testing.T) {
	t.Parallel()

	client := localClient(t)
	repo, err := dynamoassessments.NewRepository(client, createTable(t, client))
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	ctx := context.Background()

	session := sampleSession("a-1", 1)
	if err := repo.Create(ctx, "owner-1", session); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Create(ctx, "owner-1", session); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate create error = %v", err)
	}

	stored, err := repo.Get(ctx, "owner-1", "a-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Language != "ja" || len(stored.Stages) != 1 || stored.Stages[0].Items[0].CorrectIndex != 0 {
		t.Fatalf("stored = %+v", stored)
	}
	if stored.Stages[0].Answered {
		t.Fatal("fresh stage marked answered")
	}
	if _, err := repo.Get(ctx, "owner-2", "a-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-owner get error = %v", err)
	}

	answered := stored
	answered.Stages[0].Answers = []int{0}
	answered.Stages[0].Answered = true
	answered.Stages[0].Correct = 1
	answered.Stages[0].AnsweredAt = time.Date(2026, 7, 19, 10, 5, 0, 0, time.UTC)
	answered.Status = domain.StatusCompleted
	answered.EstimatedLevel = "N5"
	answered.Confidence = domain.ConfidenceHigh
	answered.Floor = true
	answered.CompletedAt = time.Date(2026, 7, 19, 10, 5, 0, 0, time.UTC)
	answered.Version = 2

	level := &outbound.ProfileLevelRecord{Language: "ja", Level: "N5", AssessmentID: "a-1", UpdatedAt: answered.CompletedAt}
	if err := repo.Save(ctx, "owner-1", answered, level); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Save(ctx, "owner-1", answered, level); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale save error = %v", err)
	}

	final, err := repo.Get(ctx, "owner-1", "a-1")
	if err != nil {
		t.Fatalf("Get after save: %v", err)
	}
	if final.Status != domain.StatusCompleted || !final.Stages[0].Answered || final.Stages[0].Correct != 1 || !final.Floor {
		t.Fatalf("final = %+v", final)
	}
	if final.Confidence != domain.ConfidenceHigh || !final.CompletedAt.Equal(answered.CompletedAt) {
		t.Fatalf("final result fields = %+v", final)
	}

	levels, err := repo.Levels(ctx, "owner-1")
	if err != nil {
		t.Fatalf("Levels: %v", err)
	}
	if len(levels) != 1 || levels[0].Level != "N5" || levels[0].AssessmentID != "a-1" {
		t.Fatalf("levels = %+v", levels)
	}

	second := sampleSession("a-2", 1)
	if err := repo.Create(ctx, "owner-1", second); err != nil {
		t.Fatalf("Create second: %v", err)
	}
	sessions, err := repo.List(ctx, "owner-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}
	other, err := repo.List(ctx, "owner-2")
	if err != nil {
		t.Fatalf("List other: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("cross-owner list = %d items", len(other))
	}
}

func TestSaveWithoutLevelKeepsProfileUntouched(t *testing.T) {
	t.Parallel()

	client := localClient(t)
	repo, err := dynamoassessments.NewRepository(client, createTable(t, client))
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	ctx := context.Background()

	session := sampleSession("a-1", 1)
	if err := repo.Create(ctx, "owner-1", session); err != nil {
		t.Fatalf("Create: %v", err)
	}
	session.Version = 2
	if err := repo.Save(ctx, "owner-1", session, nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	levels, err := repo.Levels(ctx, "owner-1")
	if err != nil {
		t.Fatalf("Levels: %v", err)
	}
	if len(levels) != 0 {
		t.Fatalf("levels = %+v, want none", levels)
	}
}
