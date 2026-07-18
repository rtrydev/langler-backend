package agenttokens

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/agenttoken"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type fakeStore struct {
	tokens  []agenttoken.Token
	hash    string
	found   agenttoken.Token
	touched time.Time
}

func (f *fakeStore) Create(_ context.Context, token agenttoken.Token, hash string) error {
	f.tokens = append(f.tokens, token)
	f.hash = hash
	return nil
}

func (f *fakeStore) List(_ context.Context, _ string) ([]agenttoken.Token, error) {
	return f.tokens, nil
}

func (f *fakeStore) Revoke(_ context.Context, _, _ string, at time.Time) error {
	f.found.RevokedAt = at
	return nil
}

func (f *fakeStore) FindByHash(_ context.Context, _ string) (agenttoken.Token, error) {
	if f.found.ID == "" {
		return agenttoken.Token{}, agenttoken.ErrNotFound
	}
	return f.found, nil
}

func (f *fakeStore) Touch(_ context.Context, _ agenttoken.Token, at time.Time) error {
	f.touched = at
	return nil
}

type fakeLimiter struct{ err error }

func (f fakeLimiter) Consume(context.Context, string, time.Time, int) error { return f.err }

func activeToken(t *testing.T, now time.Time, scopes []agenttoken.Scope) agenttoken.Token {
	t.Helper()
	token, err := agenttoken.New("token-1", "user-1", "Claude Code", "abcd", scopes, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return token
}

func TestCreateReturnsSecretOnceAndStoresOnlyHash(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{}
	service, err := NewService(store, fakeLimiter{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	service.now = func() time.Time { return now }
	next := byte(1)
	service.random = func(buffer []byte) (int, error) {
		for index := range buffer {
			buffer[index] = next
			next++
		}
		return len(buffer), nil
	}
	created, err := service.Create(context.Background(), inbound.AgentTokenCreateCommand{
		Owner: "user-1", Label: "Claude Code", Scopes: []string{"read-reference", "import-lessons"}, ExpiresAt: now.Add(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(created.Secret) < 40 || created.Secret[:8] != "lang_sk_" {
		t.Fatalf("Secret = %q", created.Secret)
	}
	if store.hash == "" || store.hash == created.Secret {
		t.Errorf("stored hash = %q", store.hash)
	}
	if store.tokens[0].Suffix != created.Secret[len(created.Secret)-4:] {
		t.Errorf("Suffix = %q", store.tokens[0].Suffix)
	}
}

func TestAuthorizeEnforcesScopeExpiryRevocationAndRateLimit(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		token   func(agenttoken.Token) agenttoken.Token
		scope   agenttoken.Scope
		limiter error
		wantErr bool
	}{
		{name: "allows scoped request", token: func(token agenttoken.Token) agenttoken.Token { return token }, scope: agenttoken.ScopeReadReference},
		{name: "rejects missing scope", token: func(token agenttoken.Token) agenttoken.Token { return token }, scope: agenttoken.ScopeImportLessons, wantErr: true},
		{name: "rejects expired", token: func(token agenttoken.Token) agenttoken.Token { token.ExpiresAt = now; return token }, scope: agenttoken.ScopeReadReference, wantErr: true},
		{name: "rejects revoked", token: func(token agenttoken.Token) agenttoken.Token { token.RevokedAt = now.Add(-time.Minute); return token }, scope: agenttoken.ScopeReadReference, wantErr: true},
		{name: "rejects rate limited", token: func(token agenttoken.Token) agenttoken.Token { return token }, scope: agenttoken.ScopeReadReference, limiter: agenttoken.ErrRateLimited, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeStore{found: test.token(activeToken(t, now, []agenttoken.Scope{agenttoken.ScopeReadReference}))}
			service, err := NewService(store, fakeLimiter{err: test.limiter})
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}
			service.now = func() time.Time { return now }
			result, err := service.Authorize(context.Background(), "lang_sk_secret", test.scope)
			if (err != nil) != test.wantErr {
				t.Fatalf("Authorize error = %v, wantErr %v", err, test.wantErr)
			}
			if !test.wantErr && (result.Owner != "user-1" || store.touched != now) {
				t.Errorf("result = %+v touched = %v", result, store.touched)
			}
		})
	}
}

func TestAuthorizeRejectsMalformedToken(t *testing.T) {
	service, err := NewService(&fakeStore{}, fakeLimiter{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.Authorize(context.Background(), "jwt", agenttoken.ScopeReadReference)
	if !errors.Is(err, agenttoken.ErrInvalidToken) {
		t.Fatalf("error = %v", err)
	}
}
