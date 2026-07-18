package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type Handler struct {
	status    inbound.StatusProvider
	reference inbound.ReferenceProvider
	importer  inbound.LessonImporter
	library   inbound.LessonLibrary
	prompts   inbound.LessonPromptBuilder
	results   inbound.LessonResultRecorder
	progress  inbound.ProgressProvider
	tokens    inbound.AgentTokenManager
}

func NewHandler(
	status inbound.StatusProvider,
	reference inbound.ReferenceProvider,
	importer inbound.LessonImporter,
	library inbound.LessonLibrary,
	prompts inbound.LessonPromptBuilder,
	results inbound.LessonResultRecorder,
	progress inbound.ProgressProvider,
	tokens inbound.AgentTokenManager,
) (*Handler, error) {
	if status == nil {
		return nil, errors.New("status provider must not be nil")
	}
	if reference == nil {
		return nil, errors.New("reference provider must not be nil")
	}
	if importer == nil {
		return nil, errors.New("lesson importer must not be nil")
	}
	if library == nil {
		return nil, errors.New("lesson library must not be nil")
	}
	if prompts == nil {
		return nil, errors.New("lesson prompt builder must not be nil")
	}
	if results == nil {
		return nil, errors.New("lesson result recorder must not be nil")
	}
	if progress == nil {
		return nil, errors.New("progress provider must not be nil")
	}
	if tokens == nil {
		return nil, errors.New("agent token manager must not be nil")
	}
	return &Handler{status: status, reference: reference, importer: importer, library: library, prompts: prompts, results: results, progress: progress, tokens: tokens}, nil
}

func (h *Handler) Handle(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	method := req.RequestContext.HTTP.Method
	path := strings.TrimSuffix(req.RawPath, "/")
	slog.InfoContext(ctx, "request", "method", method, "path", path)

	switch {
	case method == http.MethodGet && path == "/hello":
		return h.handleStatus(ctx), nil
	case method == http.MethodGet && path == "/reference/vocab":
		return h.handleVocab(ctx, req.QueryStringParameters), nil
	case method == http.MethodGet && path == "/reference/grammar":
		return h.handleGrammar(ctx, req.QueryStringParameters), nil
	case method == http.MethodGet && path == "/reference/scripts":
		return h.handleScripts(ctx, req.QueryStringParameters), nil
	case method == http.MethodPost && path == "/lessons/prompt":
		return h.handleLessonPrompt(ctx, req), nil
	case method == http.MethodPost && path == "/lessons/import":
		return h.handleLessonImport(ctx, req), nil
	case method == http.MethodPost && path == "/agent-tokens":
		return h.handleAgentTokenCreate(ctx, req), nil
	case method == http.MethodGet && path == "/agent-tokens":
		return h.handleAgentTokenList(ctx, req), nil
	case method == http.MethodDelete && strings.HasPrefix(path, "/agent-tokens/"):
		return h.handleAgentTokenRevoke(ctx, req, strings.TrimPrefix(path, "/agent-tokens/")), nil
	case method == http.MethodGet && path == "/lessons":
		return h.handleLessonList(ctx, req), nil
	case method == http.MethodGet && path == "/reviews/due":
		return h.handleDueReviews(ctx, req), nil
	case method == http.MethodPost && path == "/reviews/grade":
		return h.handleReviewGrade(ctx, req), nil
	case method == http.MethodGet && path == "/progress":
		return h.handleProgressSummary(ctx, req), nil
	case method == http.MethodPost && strings.HasPrefix(path, "/lessons/") && strings.HasSuffix(path, "/results"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/lessons/"), "/results")
		return h.handleLessonResult(ctx, req, id), nil
	case method == http.MethodGet && strings.HasPrefix(path, "/lessons/"):
		return h.handleLessonGet(ctx, req, strings.TrimPrefix(path, "/lessons/")), nil
	case method == http.MethodDelete && strings.HasPrefix(path, "/lessons/"):
		return h.handleLessonDelete(ctx, req, strings.TrimPrefix(path, "/lessons/")), nil
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
