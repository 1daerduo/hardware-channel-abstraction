package security

import (
	"context"
	"fmt"
	"sync"
)

// Identity is an authenticated subject. Principal is the stable subject id
// used for authorization; Type records its kind for audit (Design doc 10 §3).
type Identity struct {
	Principal string
	Type      string // user | service | agent | loop | cli
}

// Authenticator answers "who are you" (Design doc 10 §4). It maps a bearer
// token to an Identity. Core depends only on the unified Identity, never on
// the concrete auth mechanism.
type Authenticator struct {
	mu     sync.RWMutex
	tokens map[string]Identity
}

// NewAuthenticator builds an empty token authenticator.
func NewAuthenticator() *Authenticator {
	return &Authenticator{tokens: map[string]Identity{}}
}

// RegisterToken registers a bearer token for an identity.
func (a *Authenticator) RegisterToken(token string, id Identity) error {
	if token == "" || id.Principal == "" {
		return fmt.Errorf("authenticator: token and principal are required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokens[token] = id
	return nil
}

// Authenticate resolves a token to an Identity, or errors for an unknown
// token.
func (a *Authenticator) Authenticate(_ context.Context, token string) (Identity, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	id, ok := a.tokens[token]
	if !ok {
		return Identity{}, fmt.Errorf("authenticator: invalid token")
	}
	return id, nil
}

// RevokeToken removes a token.
func (a *Authenticator) RevokeToken(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.tokens, token)
}
