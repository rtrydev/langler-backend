package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type Handler struct {
	status inbound.StatusProvider
}

func NewHandler(status inbound.StatusProvider) (*Handler, error) {
	if status == nil {
		return nil, errors.New("status provider must not be nil")
	}
	return &Handler{status: status}, nil
}

type statusResponse struct {
	Message string `json:"message"`
	Service string `json:"service"`
	Stage   string `json:"stage"`
}

func (h *Handler) Handle(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	slog.InfoContext(ctx, "request",
		"method", req.RequestContext.HTTP.Method,
		"path", req.RawPath,
	)

	st, err := h.status.Status(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "status query failed", "error", err)
		return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusInternalServerError}, nil
	}

	body, err := json.Marshal(statusResponse{
		Message: st.Message,
		Service: st.Service,
		Stage:   st.Stage,
	})
	if err != nil {
		slog.ErrorContext(ctx, "response encoding failed", "error", err)
		return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusInternalServerError}, nil
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}, nil
}
