package agenttoken

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type Scope string

const (
	ScopeReadReference Scope = "read-reference"
	ScopeImportLessons Scope = "import-lessons"
)

var (
	ErrInvalidToken  = errors.New("invalid agent token")
	ErrNotFound      = errors.New("agent token not found")
	ErrAlreadyExists = errors.New("agent token already exists")
	ErrStorage       = errors.New("agent token storage failed")
	ErrRateLimited   = errors.New("agent token rate limit exceeded")
)

type Token struct {
	ID        string
	Owner     string
	Label     string
	Scopes    []Scope
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt time.Time
	LastUsed  time.Time
	Suffix    string
}

func New(id, owner, label, suffix string, scopes []Scope, createdAt, expiresAt time.Time) (Token, error) {
	label = strings.TrimSpace(label)
	if id == "" || owner == "" || suffix == "" || label == "" || utf8.RuneCountInString(label) > 80 || len(scopes) == 0 || len(scopes) > 2 {
		return Token{}, ErrInvalidToken
	}
	cleanScopes := make([]Scope, 0, len(scopes))
	for _, scope := range scopes {
		if (scope != ScopeReadReference && scope != ScopeImportLessons) || slices.Contains(cleanScopes, scope) {
			return Token{}, ErrInvalidToken
		}
		cleanScopes = append(cleanScopes, scope)
	}
	if createdAt.IsZero() || expiresAt.IsZero() || !expiresAt.After(createdAt) {
		return Token{}, ErrInvalidToken
	}
	return Token{
		ID: id, Owner: owner, Label: label, Suffix: suffix,
		Scopes: cleanScopes, CreatedAt: createdAt.UTC(), ExpiresAt: expiresAt.UTC(),
	}, nil
}

func (t Token) HasScope(scope Scope) bool {
	return slices.Contains(t.Scopes, scope)
}

func (t Token) Active(at time.Time) bool {
	return t.RevokedAt.IsZero() && at.Before(t.ExpiresAt)
}
