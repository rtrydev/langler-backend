package machineauth

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/aws/aws-lambda-go/events"

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
	result, err := h.authorizer.Authorize(ctx, authorization, req.RouteKey)
	if err != nil {
		slog.WarnContext(ctx, "machine request denied", "route", req.RouteKey, "error", err)
		return events.APIGatewayV2CustomAuthorizerSimpleResponse{IsAuthorized: false}, nil
	}
	return events.APIGatewayV2CustomAuthorizerSimpleResponse{
		IsAuthorized: true,
		Context:      map[string]interface{}{"owner": result.Owner, "tokenId": result.TokenID},
	}, nil
}
