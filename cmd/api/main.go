package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/httpapi"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoref"
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
	repo, err := dynamoref.NewRepository(dynamodb.NewFromConfig(cfg), os.Getenv("TABLE_NAME"))
	if err != nil {
		return nil, err
	}
	referenceSvc, err := reference.NewService(repo)
	if err != nil {
		return nil, err
	}

	return httpapi.NewHandler(statusSvc, referenceSvc)
}
