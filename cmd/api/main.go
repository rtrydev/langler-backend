package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/httpapi"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoagenttokens"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoassessments"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoglossary"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamolessons"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoprogress"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoref"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/semanticref"
	"github.com/rtrydev/langler-backend/internal/application/agenttokens"
	"github.com/rtrydev/langler-backend/internal/application/assessments"
	"github.com/rtrydev/langler-backend/internal/application/lessons"
	progressapp "github.com/rtrydev/langler-backend/internal/application/progress"
	"github.com/rtrydev/langler-backend/internal/application/reference"
	"github.com/rtrydev/langler-backend/internal/application/status"
	lessondomain "github.com/rtrydev/langler-backend/internal/domain/lesson"
	referencedomain "github.com/rtrydev/langler-backend/internal/domain/reference"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	handler, err := wire(context.Background())
	if err != nil {
		slog.Error("wiring failed", "error", err)
		os.Exit(1)
	}

	lambda.Start(handler.Handle)
}

func wire(ctx context.Context) (*httpapi.Handler, error) {
	statusSvc, err := status.NewService("langler-backend", os.Getenv("STAGE"))
	if err != nil {
		return nil, err
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	client := dynamodb.NewFromConfig(cfg)
	table := os.Getenv("TABLE_NAME")
	repo, err := dynamoref.NewRepository(client, table)
	if err != nil {
		return nil, err
	}
	referenceSvc, err := reference.NewService(repo)
	if err != nil {
		return nil, err
	}
	lessonRepo, err := dynamolessons.NewRepository(client, table)
	if err != nil {
		return nil, err
	}
	progressRepo, err := dynamoprogress.NewRepository(client, table)
	if err != nil {
		return nil, err
	}
	progressSvc, err := progressapp.NewService(progressRepo, repo)
	if err != nil {
		return nil, err
	}
	indexURLs, err := embeddingIndexURLs(os.Getenv("EMBEDDINGS_URLS"), os.Getenv("EMBEDDINGS_URL"))
	if err != nil {
		return nil, err
	}
	semantic, err := semanticref.New(bedrockruntime.NewFromConfig(cfg), indexURLs, os.Getenv("EMBED_MODEL_ID"))
	if err != nil {
		return nil, err
	}
	glossaryRepo, err := dynamoglossary.NewRepository(client, table)
	if err != nil {
		return nil, err
	}
	lessonsSvc, err := lessons.NewService(lessonRepo, repo, repo, progressRepo, semantic, lessonRepo, progressSvc, glossaryRepo)
	if err != nil {
		return nil, err
	}
	tokenRepo, err := dynamoagenttokens.NewRepository(client, table)
	if err != nil {
		return nil, err
	}
	tokenSvc, err := agenttokens.NewService(tokenRepo, tokenRepo)
	if err != nil {
		return nil, err
	}
	assessmentRepo, err := dynamoassessments.NewRepository(client, table)
	if err != nil {
		return nil, err
	}
	assessmentSvc, err := assessments.NewService(assessmentRepo, repo)
	if err != nil {
		return nil, err
	}

	return httpapi.NewHandler(statusSvc, referenceSvc, lessonsSvc, lessonsSvc, lessonsSvc, lessonsSvc, lessonsSvc, progressSvc, lessonsSvc, tokenSvc, assessmentSvc)
}

func embeddingIndexURLs(raw, legacyJapaneseURL string) (map[referencedomain.Language]string, error) {
	if raw == "" {
		urls := make(map[referencedomain.Language]string)
		if legacyJapaneseURL != "" {
			urls["ja"] = legacyJapaneseURL
		}
		return urls, nil
	}

	var configured map[string]string
	if err := json.Unmarshal([]byte(raw), &configured); err != nil {
		return nil, fmt.Errorf("parse EMBEDDINGS_URLS: %w", err)
	}
	urls := make(map[referencedomain.Language]string, len(configured))
	for language, indexURL := range configured {
		if !lessondomain.KnownLanguage(lessondomain.Language(language)) {
			return nil, fmt.Errorf("parse EMBEDDINGS_URLS language %q: unsupported language", language)
		}
		lang, err := referencedomain.NewLanguage(language)
		if err != nil {
			return nil, fmt.Errorf("parse EMBEDDINGS_URLS language %q: %w", language, err)
		}
		urls[lang] = indexURL
	}
	return urls, nil
}
