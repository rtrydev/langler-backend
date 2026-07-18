package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/domain/agenttoken"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type agentTokenDTO struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Scopes    []string `json:"scopes"`
	CreatedAt string   `json:"createdAt"`
	ExpiresAt string   `json:"expiresAt"`
	RevokedAt string   `json:"revokedAt,omitempty"`
	LastUsed  string   `json:"lastUsed,omitempty"`
	Suffix    string   `json:"suffix"`
	Status    string   `json:"status"`
}

func (h *Handler) handleAgentTokenCreate(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	body, errResp := requestBody(req)
	if errResp != nil {
		return *errResp
	}
	var doc struct {
		Label     string   `json:"label"`
		Scopes    []string `json:"scopes"`
		ExpiresAt string   `json:"expiresAt"`
	}
	if issue := decodeStrict(body, &doc, "$"); issue != nil {
		return respondJSON(http.StatusBadRequest, validationResponse{Error: "agent token validation failed", Issues: []issueDTO{*issue}})
	}
	expiresAt, err := time.Parse(time.RFC3339, doc.ExpiresAt)
	if err != nil {
		return errorJSON(http.StatusBadRequest, "expiresAt must be an RFC 3339 timestamp")
	}
	created, err := h.tokens.Create(ctx, inbound.AgentTokenCreateCommand{Owner: owner, Label: doc.Label, Scopes: doc.Scopes, ExpiresAt: expiresAt})
	if err != nil {
		return agentTokenError(ctx, err)
	}
	return respondJSON(http.StatusCreated, struct {
		Token  agentTokenDTO `json:"token"`
		Secret string        `json:"secret"`
	}{Token: toAgentTokenDTO(created.Token, time.Now().UTC()), Secret: created.Secret})
}

func (h *Handler) handleAgentTokenList(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	tokens, err := h.tokens.List(ctx, owner)
	if err != nil {
		return agentTokenError(ctx, err)
	}
	now := time.Now().UTC()
	items := make([]agentTokenDTO, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, toAgentTokenDTO(token, now))
	}
	return respondJSON(http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) handleAgentTokenRevoke(ctx context.Context, req events.APIGatewayV2HTTPRequest, id string) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	if err := h.tokens.Revoke(ctx, owner, id); err != nil {
		return agentTokenError(ctx, err)
	}
	return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusNoContent}
}

func toAgentTokenDTO(token agenttoken.Token, now time.Time) agentTokenDTO {
	status := "active"
	if !token.RevokedAt.IsZero() {
		status = "revoked"
	} else if !now.Before(token.ExpiresAt) {
		status = "expired"
	}
	scopes := make([]string, 0, len(token.Scopes))
	for _, scope := range token.Scopes {
		scopes = append(scopes, string(scope))
	}
	dto := agentTokenDTO{ID: token.ID, Label: token.Label, Scopes: scopes, CreatedAt: token.CreatedAt.Format(time.RFC3339), ExpiresAt: token.ExpiresAt.Format(time.RFC3339), Suffix: token.Suffix, Status: status}
	if !token.RevokedAt.IsZero() {
		dto.RevokedAt = token.RevokedAt.Format(time.RFC3339)
	}
	if !token.LastUsed.IsZero() {
		dto.LastUsed = token.LastUsed.Format(time.RFC3339)
	}
	return dto
}

func agentTokenError(ctx context.Context, err error) events.APIGatewayV2HTTPResponse {
	switch {
	case errors.Is(err, agenttoken.ErrInvalidToken):
		return errorJSON(http.StatusBadRequest, agenttoken.ErrInvalidToken.Error())
	case errors.Is(err, agenttoken.ErrNotFound):
		return errorJSON(http.StatusNotFound, agenttoken.ErrNotFound.Error())
	case errors.Is(err, agenttoken.ErrAlreadyExists):
		return errorJSON(http.StatusConflict, agenttoken.ErrAlreadyExists.Error())
	default:
		slog.ErrorContext(ctx, "agent token operation failed", "error", err)
		return errorJSON(http.StatusInternalServerError, "internal error")
	}
}
