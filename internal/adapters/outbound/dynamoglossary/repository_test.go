package dynamoglossary_test

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoglossary"
	domain "github.com/rtrydev/langler-backend/internal/domain/progress"
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

	table := "glossary-test-" + time.Now().UTC().Format("20060102150405.000000000")
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

func TestLessonWordLifecycle(t *testing.T) {
	client := localClient(t)
	table := createTable(t, client)
	repo, err := dynamoglossary.NewRepository(client, table)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	ctx := context.Background()
	refs := outbound.GlossaryRefs{VocabIDs: []string{"N4#1416220", "N4#1311125"}, GrammarIDs: []string{"N4#volitional"}}
	addedAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	if err := repo.AddLessonWords(ctx, "user-1", "ja", "lesson-1", refs, addedAt); err != nil {
		t.Fatalf("AddLessonWords: %v", err)
	}
	// A second lesson shares one word; re-adding the first lesson is idempotent.
	overlap := outbound.GlossaryRefs{VocabIDs: []string{"N4#1416220"}}
	if err := repo.AddLessonWords(ctx, "user-1", "ja", "lesson-2", overlap, addedAt.Add(time.Hour)); err != nil {
		t.Fatalf("AddLessonWords lesson-2: %v", err)
	}
	if err := repo.AddLessonWords(ctx, "user-1", "ja", "lesson-1", refs, addedAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("AddLessonWords replay: %v", err)
	}

	entries, err := repo.Entries(ctx, "user-1", "ja", domain.KindVocab)
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want 2 vocab words", entries)
	}
	byID := map[string]outbound.GlossaryEntry{}
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	if byID["N4#1416220"].LessonCount != 2 || byID["N4#1311125"].LessonCount != 1 {
		t.Fatalf("lesson counts = %+v", byID)
	}
	if !byID["N4#1416220"].AddedAt.Equal(addedAt) {
		t.Errorf("addedAt = %v, want first add time to stick", byID["N4#1416220"].AddedAt)
	}

	grammarIDs, err := repo.GlossaryItemIDs(ctx, "user-1", "ja", domain.KindGrammar)
	if err != nil {
		t.Fatalf("GlossaryItemIDs: %v", err)
	}
	if !slices.Equal(grammarIDs, []string{"N4#volitional"}) {
		t.Fatalf("grammar ids = %v", grammarIDs)
	}

	if err := repo.RemoveLessonWords(ctx, "user-1", "ja", "lesson-1", refs); err != nil {
		t.Fatalf("RemoveLessonWords: %v", err)
	}
	vocabIDs, err := repo.GlossaryItemIDs(ctx, "user-1", "ja", domain.KindVocab)
	if err != nil {
		t.Fatalf("GlossaryItemIDs after remove: %v", err)
	}
	if !slices.Equal(vocabIDs, []string{"N4#1416220"}) {
		t.Fatalf("vocab ids after remove = %v, want only the shared word", vocabIDs)
	}
	// Removing words for a lesson that never contributed is a no-op.
	if err := repo.RemoveLessonWords(ctx, "user-1", "ja", "lesson-3", refs); err != nil {
		t.Fatalf("RemoveLessonWords unknown lesson: %v", err)
	}
	if err := repo.RemoveLessonWords(ctx, "user-1", "ja", "lesson-2", overlap); err != nil {
		t.Fatalf("RemoveLessonWords lesson-2: %v", err)
	}
	vocabIDs, err = repo.GlossaryItemIDs(ctx, "user-1", "ja", domain.KindVocab)
	if err != nil {
		t.Fatalf("GlossaryItemIDs final: %v", err)
	}
	if len(vocabIDs) != 0 {
		t.Fatalf("vocab ids = %v, want empty glossary", vocabIDs)
	}
}
