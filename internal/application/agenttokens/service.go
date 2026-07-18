package agenttokens

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/agenttoken"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

type Service struct {
	store   outbound.AgentTokenStore
	limiter outbound.AgentTokenRateLimiter
	now     func() time.Time
	random  func([]byte) (int, error)
}

func NewService(store outbound.AgentTokenStore, limiter outbound.AgentTokenRateLimiter) (*Service, error) {
	if store == nil {
		return nil, errors.New("agent token store must not be nil")
	}
	if limiter == nil {
		return nil, errors.New("agent token rate limiter must not be nil")
	}
	return &Service{store: store, limiter: limiter, now: time.Now, random: rand.Read}, nil
}

func (s *Service) Create(ctx context.Context, command inbound.AgentTokenCreateCommand) (inbound.AgentTokenCreated, error) {
	if command.Owner == "" {
		return inbound.AgentTokenCreated{}, agenttoken.ErrInvalidToken
	}
	now := s.now().UTC()
	scopes := make([]agenttoken.Scope, 0, len(command.Scopes))
	for _, scope := range command.Scopes {
		scopes = append(scopes, agenttoken.Scope(scope))
	}
	id, err := s.identifier()
	if err != nil {
		return inbound.AgentTokenCreated{}, err
	}
	secretBytes := make([]byte, 32)
	if _, err := s.random(secretBytes); err != nil {
		return inbound.AgentTokenCreated{}, fmt.Errorf("generate agent token secret: %w", err)
	}
	secret := agenttoken.SecretPrefix + base64.RawURLEncoding.EncodeToString(secretBytes)
	token, err := agenttoken.New(id, command.Owner, command.Label, secret[len(secret)-4:], scopes, now, command.ExpiresAt)
	if err != nil {
		return inbound.AgentTokenCreated{}, err
	}
	hash := sha256.Sum256([]byte(secret))
	if err := s.store.Create(ctx, token, hex.EncodeToString(hash[:])); err != nil {
		return inbound.AgentTokenCreated{}, err
	}
	return inbound.AgentTokenCreated{Token: token, Secret: secret}, nil
}

func (s *Service) identifier() (string, error) {
	raw := make([]byte, 16)
	if _, err := s.random(raw); err != nil {
		return "", fmt.Errorf("generate agent token id: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(raw)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}

func (s *Service) List(ctx context.Context, owner string) ([]agenttoken.Token, error) {
	if owner == "" {
		return nil, agenttoken.ErrInvalidToken
	}
	return s.store.List(ctx, owner)
}

func (s *Service) Revoke(ctx context.Context, owner, id string) error {
	if owner == "" || id == "" {
		return agenttoken.ErrInvalidToken
	}
	return s.store.Revoke(ctx, owner, id, s.now().UTC())
}

func (s *Service) Authorize(ctx context.Context, secret string, requiredScope agenttoken.Scope) (inbound.MachineAuthorization, error) {
	if len(secret) <= len(agenttoken.SecretPrefix) || !strings.HasPrefix(secret, agenttoken.SecretPrefix) {
		return inbound.MachineAuthorization{}, agenttoken.ErrInvalidToken
	}
	hash := sha256.Sum256([]byte(secret))
	token, err := s.store.FindByHash(ctx, hex.EncodeToString(hash[:]))
	if err != nil {
		return inbound.MachineAuthorization{}, err
	}
	now := s.now().UTC()
	if !token.Active(now) {
		return inbound.MachineAuthorization{}, agenttoken.ErrInvalidToken
	}
	if !token.HasScope(requiredScope) {
		return inbound.MachineAuthorization{}, agenttoken.ErrInvalidToken
	}
	if err := s.limiter.Consume(ctx, token.ID, now.Truncate(time.Minute), agenttoken.RequestsPerMinute); err != nil {
		return inbound.MachineAuthorization{}, err
	}
	if err := s.store.Touch(ctx, token, now); err != nil {
		return inbound.MachineAuthorization{}, err
	}
	return inbound.MachineAuthorization{Owner: token.Owner, TokenID: token.ID}, nil
}
