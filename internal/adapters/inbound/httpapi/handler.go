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
	status    inbound.StatusProvider
	reference inbound.ReferenceProvider
}

func NewHandler(status inbound.StatusProvider, reference inbound.ReferenceProvider) (*Handler, error) {
	if status == nil {
		return nil, errors.New("status provider must not be nil")
	}
	if reference == nil {
		return nil, errors.New("reference provider must not be nil")
	}
	return &Handler{status: status, reference: reference}, nil
}

func (h *Handler) Handle(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	method := req.RequestContext.HTTP.Method
	slog.InfoContext(ctx, "request", "method", method, "path", req.RawPath)

	if method != http.MethodGet {
		return errorJSON(http.StatusMethodNotAllowed, "method not allowed"), nil
	}

	switch req.RawPath {
	case "/hello":
		return h.handleStatus(ctx), nil
	case "/reference/vocab":
		return h.handleVocab(ctx, req.QueryStringParameters), nil
	case "/reference/grammar":
		return h.handleGrammar(ctx, req.QueryStringParameters), nil
	case "/reference/scripts":
		return h.handleScripts(ctx, req.QueryStringParameters), nil
	default:
		return errorJSON(http.StatusNotFound, "not found"), nil
	}
}

type statusResponse struct {
	Message string `json:"message"`
	Service string `json:"service"`
	Stage   string `json:"stage"`
}

func (h *Handler) handleStatus(ctx context.Context) events.APIGatewayV2HTTPResponse {
	st, err := h.status.Status(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "status query failed", "error", err)
		return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusInternalServerError}
	}
	return respondJSON(http.StatusOK, statusResponse{
		Message: st.Message,
		Service: st.Service,
		Stage:   st.Stage,
	})
}

func respondJSON(status int, payload any) events.APIGatewayV2HTTPResponse {
	body, err := json.Marshal(payload)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusInternalServerError}
	}
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}
}

func errorJSON(status int, message string) events.APIGatewayV2HTTPResponse {
	return respondJSON(status, map[string]string{"error": message})
}
