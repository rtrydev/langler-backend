package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/machineauth"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoagenttokens"
	"github.com/rtrydev/langler-backend/internal/application/agenttokens"
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

func wire(ctx context.Context) (*machineauth.Handler, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	repo, err := dynamoagenttokens.NewRepository(dynamodb.NewFromConfig(cfg), os.Getenv("TABLE_NAME"))
	if err != nil {
		return nil, err
	}
	service, err := agenttokens.NewService(repo, repo)
	if err != nil {
		return nil, err
	}
	return machineauth.NewHandler(service)
}
