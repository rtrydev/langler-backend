package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/httpapi"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoagenttokens"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoassessments"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamolessons"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoprogress"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoref"
	"github.com/rtrydev/langler-backend/internal/application/agenttokens"
	"github.com/rtrydev/langler-backend/internal/application/assessments"
	"github.com/rtrydev/langler-backend/internal/application/lessons"
	progressapp "github.com/rtrydev/langler-backend/internal/application/progress"
	"github.com/rtrydev/langler-backend/internal/application/reference"
	"github.com/rtrydev/langler-backend/internal/application/status"
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
	lessonsSvc, err := lessons.NewService(lessonRepo, repo, repo, lessonRepo, progressSvc)
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

	return httpapi.NewHandler(statusSvc, referenceSvc, lessonsSvc, lessonsSvc, lessonsSvc, lessonsSvc, progressSvc, tokenSvc, assessmentSvc)
}
