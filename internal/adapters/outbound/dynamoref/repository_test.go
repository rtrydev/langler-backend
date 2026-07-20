package dynamoref_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoref"
	domain "github.com/rtrydev/langler-backend/internal/domain/reference"
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

	table := "reference-test-" + time.Now().UTC().Format("20060102150405.000000000")
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

func putItems(t *testing.T, client *dynamodb.Client, table string, items []map[string]any) {
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

func seedReferenceData(t *testing.T, client *dynamodb.Client, table string) {
	t.Helper()

	putItems(t, client, table, []map[string]any{
		{
			"PK": "REF#ja", "SK": "VOCAB#N5#1000001", "lang": "ja",
			"headword": "学校", "reading": "がっこう",
			"gloss": []string{"school"}, "pos": []string{"n"},
			"level": "N5", "freqBand": 2, "topics": []string{"daily-routines"},
			"example": map[string]any{
				"text": "学校に行きます。", "translation": "I go to school.",
				"sourceId": "tatoeba", "license": "CC BY 2.0 FR",
			},
			"sourceId": "jmdict-simplified", "license": "CC BY-SA 4.0 (EDRDG)",
		},
		{
			"PK": "REF#ja", "SK": "VOCAB#N5#1000002", "lang": "ja",
			"headword": "犬", "reading": "いぬ",
			"gloss": []string{"dog"}, "pos": []string{"n"},
			"level": "N5", "topics": []string{},
			"sourceId": "jmdict-simplified", "license": "CC BY-SA 4.0 (EDRDG)",
		},
		{
			"PK": "REF#ja", "SK": "VOCAB#N4#1000003", "lang": "ja",
			"headword": "経験", "reading": "けいけん",
			"gloss": []string{"experience"}, "pos": []string{"n", "vs"},
			"level": "N4", "topics": []string{},
			"sourceId": "jmdict-simplified", "license": "CC BY-SA 4.0 (EDRDG)",
		},
		{
			"PK": "REF#ja", "SK": "TOPIC#N5#daily-routines", "lang": "ja",
			"slug": "daily-routines", "name": "Daily routines",
			"description": "Everyday routines, actions, and common objects",
			"level":       "N5", "vocabIds": []string{"N5#1000001"},
			"sourceId": "langler-curated", "license": "CC BY-SA 4.0",
		},
		{
			"PK": "REF#ja", "SK": "TOPIC#N5#nature-animals", "lang": "ja",
			"slug": "nature-animals", "name": "Nature & animals",
			"description": "Weather, seasons, animals, plants, and landscapes",
			"level":       "N5", "vocabIds": []string{"N5#1000002"},
			"sourceId": "langler-curated", "license": "CC BY-SA 4.0",
		},
		{
			"PK": "REF#ja", "SK": "TOPIC#N4#time-numbers", "lang": "ja",
			"slug": "time-numbers", "name": "Time & numbers",
			"description": "Ideas, change, degree, and ways of thinking",
			"level":       "N4", "vocabIds": []string{"N4#1000003"},
			"sourceId": "langler-curated", "license": "CC BY-SA 4.0",
		},
		{
			"PK": "REF#ja", "SK": "GRAMMAR#N5#particle-wa", "lang": "ja",
			"topicId": "particle-wa", "name": "Topic particle は", "level": "N5",
			"description": "Marks the topic of the sentence.",
			"example":     map[string]any{"text": "私は学生です。", "translation": "I am a student."},
			"sourceId":    "langler-curated", "license": "CC BY-SA 4.0",
		},
		{
			"PK": "REF#pl", "SK": "GRAMMAR#A1#present", "lang": "pl",
			"topicId": "present", "name": "Present", "level": "A1",
			"description": "Present tense.",
			"example":     map[string]any{"text": "Czytam.", "translation": "I read."},
			"sourceId":    "certyfikat-polish", "license": "Official legal text",
		},
		{
			"PK": "REF#pl", "SK": "GRAMMAR#A2#imperative", "lang": "pl",
			"topicId": "imperative", "name": "Imperative", "level": "A2",
			"description": "Imperative mood.",
			"example":     map[string]any{"text": "Czytaj!", "translation": "Read!"},
			"sourceId":    "certyfikat-polish", "license": "Official legal text",
		},
		{
			"PK": "REF#pl", "SK": "GRAMMAR#B1#relative", "lang": "pl",
			"topicId": "relative", "name": "Relative", "level": "B1",
			"description": "Relative clauses.",
			"example":     map[string]any{"text": "Książka, którą czytam.", "translation": "The book I read."},
			"sourceId":    "certyfikat-polish", "license": "Official legal text",
		},
		{
			"PK": "REF#ja", "SK": "SCRIPT#KANA#H001", "lang": "ja",
			"glyph": "あ", "scriptType": "kana", "name": "hiragana a",
			"readings": map[string][]string{"romaji": {"a"}}, "kanaScript": "hiragana",
			"sourceId": "langler-curated", "license": "CC0",
		},
		{
			"PK": "REF#ja", "SK": "SCRIPT#KANJI#N5#犬", "lang": "ja",
			"glyph": "犬", "scriptType": "kanji", "name": "dog",
			"meanings": []string{"dog"},
			"readings": map[string][]string{"on": {"ケン"}, "kun": {"いぬ"}},
			"level":    "N5", "grade": 1, "strokeCount": 4,
			"strokeDataRef": "kanjivg/072ac.svg", "components": []string{"大", "丶"},
			"sourceId": "kanjidic2", "license": "CC BY-SA 4.0 (EDRDG)",
		},
	})
}

func newRepository(t *testing.T) *dynamoref.Repository {
	t.Helper()

	client := localClient(t)
	table := createTable(t, client)
	seedReferenceData(t, client, table)

	repo, err := dynamoref.NewRepository(client, table)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	return repo
}

func TestVocabQueries(t *testing.T) {
	repo := newRepository(t)
	ctx := context.Background()

	t.Run("level scoped", func(t *testing.T) {
		page, err := repo.Vocab(ctx, outbound.VocabFilter{Language: "ja", Level: "N5", Limit: 10})
		if err != nil {
			t.Fatalf("Vocab: %v", err)
		}
		if len(page.Entries) != 2 {
			t.Fatalf("entries = %d, want 2", len(page.Entries))
		}
		first := page.Entries[0]
		if first.Headword != "学校" || first.Reading != "がっこう" || first.Level != "N5" {
			t.Errorf("first entry = %+v, want 学校/がっこう/N5", first)
		}
		if first.Example == nil || first.Example.Text != "学校に行きます。" {
			t.Errorf("example = %+v, want the seeded sentence", first.Example)
		}
		if first.FreqBand != 2 {
			t.Errorf("freqBand = %d, want 2", first.FreqBand)
		}
	})

	t.Run("all levels", func(t *testing.T) {
		page, err := repo.Vocab(ctx, outbound.VocabFilter{Language: "ja", Limit: 10})
		if err != nil {
			t.Fatalf("Vocab: %v", err)
		}
		if len(page.Entries) != 3 {
			t.Fatalf("entries = %d, want 3", len(page.Entries))
		}
	})

	t.Run("topic filter", func(t *testing.T) {
		page, err := repo.Vocab(ctx, outbound.VocabFilter{Language: "ja", Level: "N5", Topic: "daily-routines", Limit: 10})
		if err != nil {
			t.Fatalf("Vocab: %v", err)
		}
		if len(page.Entries) != 1 || page.Entries[0].Headword != "学校" {
			t.Fatalf("entries = %+v, want only 学校", page.Entries)
		}
	})

	t.Run("pagination round trip", func(t *testing.T) {
		first, err := repo.Vocab(ctx, outbound.VocabFilter{Language: "ja", Level: "N5", Limit: 1})
		if err != nil {
			t.Fatalf("Vocab page 1: %v", err)
		}
		if len(first.Entries) != 1 || first.NextCursor == "" {
			t.Fatalf("page 1 = %d entries, cursor %q; want 1 entry and a cursor", len(first.Entries), first.NextCursor)
		}
		second, err := repo.Vocab(ctx, outbound.VocabFilter{Language: "ja", Level: "N5", Limit: 10, Cursor: first.NextCursor})
		if err != nil {
			t.Fatalf("Vocab page 2: %v", err)
		}
		if len(second.Entries) != 1 {
			t.Fatalf("page 2 entries = %d, want 1", len(second.Entries))
		}
		if second.Entries[0].Headword == first.Entries[0].Headword {
			t.Fatalf("page 2 repeated %q", first.Entries[0].Headword)
		}
	})

	t.Run("invalid cursor", func(t *testing.T) {
		_, err := repo.Vocab(ctx, outbound.VocabFilter{Language: "ja", Limit: 10, Cursor: "not-a-cursor"})
		if !errors.Is(err, domain.ErrInvalidCursor) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidCursor)
		}
	})

	t.Run("cursor scoped to query", func(t *testing.T) {
		first, err := repo.Vocab(ctx, outbound.VocabFilter{Language: "ja", Level: "N5", Limit: 1})
		if err != nil {
			t.Fatalf("Vocab: %v", err)
		}
		_, err = repo.Vocab(ctx, outbound.VocabFilter{Language: "ja", Level: "N4", Limit: 1, Cursor: first.NextCursor})
		if !errors.Is(err, domain.ErrInvalidCursor) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidCursor)
		}
	})

	t.Run("unknown language is empty", func(t *testing.T) {
		page, err := repo.Vocab(ctx, outbound.VocabFilter{Language: "pl", Limit: 10})
		if err != nil {
			t.Fatalf("Vocab: %v", err)
		}
		if len(page.Entries) != 0 || page.NextCursor != "" {
			t.Fatalf("page = %+v, want empty", page)
		}
	})
}

func TestGrammarQueries(t *testing.T) {
	repo := newRepository(t)

	page, err := repo.Grammar(context.Background(), outbound.GrammarFilter{Language: "ja", Level: "N5", Limit: 10})
	if err != nil {
		t.Fatalf("Grammar: %v", err)
	}
	if len(page.Topics) != 1 {
		t.Fatalf("topics = %d, want 1", len(page.Topics))
	}
	topic := page.Topics[0]
	if topic.TopicID != "particle-wa" || topic.Name != "Topic particle は" {
		t.Errorf("topic = %+v, want particle-wa", topic)
	}
	if topic.Example == nil || topic.Example.Translation != "I am a student." {
		t.Errorf("example = %+v, want the seeded example", topic.Example)
	}

	polish, err := repo.Grammar(context.Background(), outbound.GrammarFilter{Language: "pl", Level: "A2", Limit: 10})
	if err != nil {
		t.Fatalf("Polish Grammar: %v", err)
	}
	if len(polish.Topics) != 2 || polish.Topics[0].Level != "A2" || polish.Topics[1].Level != "A1" {
		t.Fatalf("Polish topics = %+v, want target-first cumulative A2 and A1 topics", polish.Topics)
	}
}

func TestScriptQueries(t *testing.T) {
	repo := newRepository(t)
	ctx := context.Background()

	t.Run("all scripts", func(t *testing.T) {
		page, err := repo.Scripts(ctx, outbound.ScriptFilter{Language: "ja", Limit: 10})
		if err != nil {
			t.Fatalf("Scripts: %v", err)
		}
		if len(page.Glyphs) != 2 {
			t.Fatalf("glyphs = %d, want 2", len(page.Glyphs))
		}
	})

	t.Run("kana only", func(t *testing.T) {
		page, err := repo.Scripts(ctx, outbound.ScriptFilter{Language: "ja", ScriptType: "kana", Limit: 10})
		if err != nil {
			t.Fatalf("Scripts: %v", err)
		}
		if len(page.Glyphs) != 1 || page.Glyphs[0].Glyph != "あ" {
			t.Fatalf("glyphs = %+v, want あ", page.Glyphs)
		}
		if got := page.Glyphs[0].Readings["romaji"]; len(got) != 1 || got[0] != "a" {
			t.Errorf("romaji readings = %v, want [a]", got)
		}
	})

	t.Run("kanji by level", func(t *testing.T) {
		page, err := repo.Scripts(ctx, outbound.ScriptFilter{Language: "ja", ScriptType: "kanji", Level: "N5", Limit: 10})
		if err != nil {
			t.Fatalf("Scripts: %v", err)
		}
		if len(page.Glyphs) != 1 {
			t.Fatalf("glyphs = %d, want 1", len(page.Glyphs))
		}
		kanji := page.Glyphs[0]
		if kanji.Glyph != "犬" || kanji.StrokeCount != 4 || kanji.StrokeDataRef != "kanjivg/072ac.svg" {
			t.Errorf("kanji = %+v, want seeded 犬", kanji)
		}
		if len(kanji.Components) != 2 {
			t.Errorf("components = %v, want 2 entries", kanji.Components)
		}
	})
}

func TestTopicQueries(t *testing.T) {
	repo := newRepository(t)
	ctx := context.Background()

	t.Run("level scoped", func(t *testing.T) {
		topics, err := repo.Topics(ctx, outbound.TopicFilter{Language: "ja", Level: "N5"})
		if err != nil {
			t.Fatalf("Topics: %v", err)
		}
		if len(topics) != 2 {
			t.Fatalf("topics = %+v, want 2", topics)
		}
		if topics[0].Slug != "daily-routines" || topics[0].Name != "Daily routines" || topics[0].Level != "N5" {
			t.Errorf("topic = %+v", topics[0])
		}
		if len(topics[0].VocabIDs) != 1 || topics[0].VocabIDs[0] != "N5#1000001" {
			t.Errorf("vocabIds = %v", topics[0].VocabIDs)
		}
	})

	t.Run("slug scoped", func(t *testing.T) {
		topics, err := repo.Topics(ctx, outbound.TopicFilter{Language: "ja", Level: "N5", Slug: "nature-animals"})
		if err != nil {
			t.Fatalf("Topics: %v", err)
		}
		if len(topics) != 1 || topics[0].Slug != "nature-animals" {
			t.Fatalf("topics = %+v", topics)
		}
	})

	t.Run("unknown slug", func(t *testing.T) {
		topics, err := repo.Topics(ctx, outbound.TopicFilter{Language: "ja", Level: "N5", Slug: "space-travel"})
		if err != nil {
			t.Fatalf("Topics: %v", err)
		}
		if len(topics) != 0 {
			t.Fatalf("topics = %+v, want none", topics)
		}
	})
}

func TestVocabByIDs(t *testing.T) {
	repo := newRepository(t)
	ctx := context.Background()

	entries, err := repo.VocabByIDs(ctx, "ja", []string{"N4#1000003", "N5#1000001", "N5#missing"})
	if err != nil {
		t.Fatalf("VocabByIDs: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want 2", entries)
	}
	if entries[0].ID != "N4#1000003" || entries[1].ID != "N5#1000001" {
		t.Errorf("order = %q, %q; want input order", entries[0].ID, entries[1].ID)
	}
	if entries[1].Headword != "学校" || entries[1].Topics[0] != "daily-routines" {
		t.Errorf("entry = %+v", entries[1])
	}
}
