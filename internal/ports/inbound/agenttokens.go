package inbound

import (
	"context"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/agenttoken"
)

type AgentTokenCreateCommand struct {
	Owner     string
	Label     string
	Scopes    []string
	ExpiresAt time.Time
}

type AgentTokenCreated struct {
	Token  agenttoken.Token
	Secret string
}

type AgentTokenManager interface {
	Create(ctx context.Context, command AgentTokenCreateCommand) (AgentTokenCreated, error)
	List(ctx context.Context, owner string) ([]agenttoken.Token, error)
	Revoke(ctx context.Context, owner, id string) error
}

type MachineAuthorization struct {
	Owner   string
	TokenID string
}

type MachineAuthorizer interface {
	Authorize(ctx context.Context, authorization, routeKey string) (MachineAuthorization, error)
}
