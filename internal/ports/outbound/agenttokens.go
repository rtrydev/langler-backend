package outbound

import (
	"context"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/agenttoken"
)

type AgentTokenStore interface {
	Create(ctx context.Context, token agenttoken.Token, hash string) error
	List(ctx context.Context, owner string) ([]agenttoken.Token, error)
	Revoke(ctx context.Context, owner, id string, at time.Time) error
	FindByHash(ctx context.Context, hash string) (agenttoken.Token, error)
	Touch(ctx context.Context, token agenttoken.Token, at time.Time) error
}

type AgentTokenRateLimiter interface {
	Consume(ctx context.Context, tokenID string, window time.Time, limit int) error
}
