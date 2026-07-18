package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type response struct {
	Message string `json:"message"`
	Service string `json:"service"`
	Stage   string `json:"stage"`
}

func handle(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	slog.InfoContext(ctx, "request",
		"method", req.RequestContext.HTTP.Method,
		"path", req.RawPath,
	)

	body, err := json.Marshal(response{
		Message: "Hello from Langler",
		Service: "langler-backend",
		Stage:   os.Getenv("STAGE"),
	})
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusInternalServerError}, err
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}, nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	lambda.Start(handle)
}
