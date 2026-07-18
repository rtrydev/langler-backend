package dynamolessons_test

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

	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamolessons"
	domain "github.com/rtrydev/langler-backend/internal/domain/lesson"
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

	table := "lessons-test-" + time.Now().UTC().Format("20060102150405.000000000")
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

func sampleRecord(owner, id string) outbound.LessonRecord {
	return outbound.LessonRecord{
		Owner:       owner,
		ContentHash: "hash-1",
		CreatedAt:   time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
		Lesson: domain.Lesson{
			SchemaVersion: domain.SchemaVersion,
			ID:            id,
			Language:      "ja",
			Level:         "N4",
			Title:         "Weekend plans in Kyoto",
			ReadingStage:  domain.StageConnected,
			Exercises: []domain.Exercise{
				{
					ID:              "ex-1",
					Type:            domain.TypeCloze,
					Points:          8,
					ReferencedVocab: []string{"N4#1416220"},
					Cloze: &domain.Cloze{
						Text:   "先週の{{1}}に行きました。",
						Blanks: []domain.Blank{{Index: 1, Answer: "週末", Hint: "two kanji"}},
					},
				},
				{
					ID:   "ex-2",
					Type: domain.TypeReading,
					Reading: &domain.Reading{
						Genre:   domain.GenreShortStory,
						Title:   "京都の週末",
						Passage: "先週の週末、京都へ行きました。",
						Questions: []domain.Question{
							{Question: "どこですか。", Kind: domain.KindMultipleChoice, Options: []string{"京都", "大阪"}, Answer: "京都"},
						},
					},
				},
			},
		},
	}
}

func TestRepositoryRoundTrip(t *testing.T) {
	client := localClient(t)
	table := createTable(t, client)
	repo, err := dynamolessons.NewRepository(client, table)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	ctx := context.Background()

	const id = "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00"
	record := sampleRecord("user-1", id)
	if err := repo.Save(ctx, record); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := repo.Save(ctx, record); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("second Save error = %v, want ErrAlreadyExists", err)
	}

	got, err := repo.Get(ctx, "user-1", id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Lesson.Title != record.Lesson.Title || got.ContentHash != "hash-1" {
		t.Errorf("Get = %+v", got)
	}
	if !got.CreatedAt.Equal(record.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, record.CreatedAt)
	}
	if got.Lesson.Exercises[0].Cloze == nil || got.Lesson.Exercises[0].Cloze.Blanks[0].Hint != "two kanji" {
		t.Errorf("cloze payload = %+v", got.Lesson.Exercises[0].Cloze)
	}
	if got.Lesson.Exercises[1].Reading == nil || got.Lesson.Exercises[1].Reading.Questions[0].Answer != "京都" {
		t.Errorf("reading payload = %+v", got.Lesson.Exercises[1].Reading)
	}

	if _, err := repo.Get(ctx, "user-2", id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-owner Get error = %v, want ErrNotFound", err)
	}

	page, err := repo.List(ctx, "user-1", 10, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Records) != 1 || page.NextCursor != "" {
		t.Errorf("List = %+v", page)
	}

	if err := repo.Delete(ctx, "user-1", id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := repo.Delete(ctx, "user-1", id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second Delete error = %v, want ErrNotFound", err)
	}
	if _, err := repo.Get(ctx, "user-1", id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get after delete error = %v, want ErrNotFound", err)
	}
}

func TestListPaginates(t *testing.T) {
	client := localClient(t)
	table := createTable(t, client)
	repo, err := dynamolessons.NewRepository(client, table)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	ctx := context.Background()

	ids := []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
	}
	for _, id := range ids {
		if err := repo.Save(ctx, sampleRecord("user-1", id)); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}

	seen := map[string]bool{}
	cursor := ""
	for {
		page, err := repo.List(ctx, "user-1", 2, cursor)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		for _, record := range page.Records {
			seen[record.Lesson.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 3 {
		t.Errorf("saw %d lessons, want 3", len(seen))
	}

	if _, err := repo.List(ctx, "user-1", 2, "!!!"); !errors.Is(err, domain.ErrInvalidCursor) {
		t.Errorf("bad cursor error = %v, want ErrInvalidCursor", err)
	}
}
