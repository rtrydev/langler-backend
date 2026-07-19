package semanticref_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/rtrydev/langler-backend/internal/adapters/outbound/semanticref"
	domain "github.com/rtrydev/langler-backend/internal/domain/reference"
)

func TestNewRejectsNilClient(t *testing.T) {
	t.Parallel()

	if _, err := semanticref.New(nil, "ja", "http://example.com/index", "model"); err == nil {
		t.Fatal("New(nil client) error = nil")
	}
}

func TestSimilarVocabIDsRequiresConfiguration(t *testing.T) {
	t.Parallel()

	client := bedrockruntime.New(bedrockruntime.Options{Region: "eu-central-1"})
	tests := []struct {
		name     string
		indexURL string
		modelID  string
		language string
	}{
		{name: "missing index url", indexURL: "", modelID: "model", language: "ja"},
		{name: "missing model id", indexURL: "http://example.com/i", modelID: "", language: "ja"},
		{name: "other language", indexURL: "http://example.com/i", modelID: "model", language: "pl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			search, err := semanticref.New(client, "ja", tt.indexURL, tt.modelID)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			_, err = search.SimilarVocabIDs(context.Background(), domain.Language(tt.language), "N5", "a trip", 10)
			if !errors.Is(err, semanticref.ErrNotConfigured) {
				t.Fatalf("error = %v, want ErrNotConfigured", err)
			}
		})
	}
}

func TestSimilarVocabIDsAgainstBedrockAndBuiltIndex(t *testing.T) {
	indexFile := os.Getenv("SEMANTIC_INDEX_FILE")
	if indexFile == "" {
		t.Skip("set SEMANTIC_INDEX_FILE to the built .embed index (needs AWS credentials with bedrock:InvokeModel)")
	}
	raw, err := os.ReadFile(indexFile)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("eu-central-1"))
	if err != nil {
		t.Fatalf("aws config: %v", err)
	}
	search, err := semanticref.New(bedrockruntime.NewFromConfig(cfg), "ja", server.URL, "cohere.embed-multilingual-v3")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, topic := range []string{"a trip to Kyoto by train", "京都へ電車で旅行する"} {
		ids, err := search.SimilarVocabIDs(context.Background(), "ja", "N5", topic, 15)
		if err != nil {
			t.Fatalf("SimilarVocabIDs(%q): %v", topic, err)
		}
		if len(ids) < 10 {
			t.Fatalf("SimilarVocabIDs(%q) = %d ids, want >= 10", topic, len(ids))
		}
		for _, id := range ids {
			if !strings.HasPrefix(id, "N5#") {
				t.Fatalf("id %q escaped the requested level", id)
			}
		}
		t.Logf("%q -> %v", topic, ids)
	}
}
