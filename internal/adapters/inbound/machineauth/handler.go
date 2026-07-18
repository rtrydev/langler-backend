package machineauth

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/domain/agenttoken"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type Handler struct {
	authorizer inbound.MachineAuthorizer
}

func NewHandler(authorizer inbound.MachineAuthorizer) (*Handler, error) {
	if authorizer == nil {
		return nil, errors.New("machine authorizer must not be nil")
	}
	return &Handler{authorizer: authorizer}, nil
}

func (h *Handler) Handle(ctx context.Context, req events.APIGatewayV2CustomAuthorizerV2Request) (events.APIGatewayV2CustomAuthorizerSimpleResponse, error) {
	authorization := ""
	for key, value := range req.Headers {
		if strings.EqualFold(key, "Authorization") {
			authorization = value
			break
		}
	}
	secret, ok := strings.CutPrefix(strings.TrimSpace(authorization), "Bearer ")
	requiredScope, scoped := requiredScope(req.RouteKey)
	if !ok || !scoped {
		return events.APIGatewayV2CustomAuthorizerSimpleResponse{IsAuthorized: false}, nil
	}
	result, err := h.authorizer.Authorize(ctx, secret, requiredScope)
	if err != nil {
		slog.WarnContext(ctx, "machine request denied", "route", req.RouteKey, "error", err)
		return events.APIGatewayV2CustomAuthorizerSimpleResponse{IsAuthorized: false}, nil
	}
	return events.APIGatewayV2CustomAuthorizerSimpleResponse{
		IsAuthorized: true,
		Context:      map[string]interface{}{"owner": result.Owner, "tokenId": result.TokenID},
	}, nil
}

func requiredScope(routeKey string) (agenttoken.Scope, bool) {
	switch routeKey {
	case "GET /reference/vocab", "GET /reference/grammar", "GET /reference/scripts":
		return agenttoken.ScopeReadReference, true
	case "POST /lessons/import":
		return agenttoken.ScopeImportLessons, true
	default:
		return "", false
	}
}
