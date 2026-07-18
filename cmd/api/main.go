package main

import (
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/httpapi"
	"github.com/rtrydev/langler-backend/internal/application/status"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	svc, err := status.NewService("langler-backend", os.Getenv("STAGE"))
	if err != nil {
		slog.Error("wiring failed", "error", err)
		os.Exit(1)
	}

	lambda.Start(httpapi.NewHandler(svc).Handle)
}
