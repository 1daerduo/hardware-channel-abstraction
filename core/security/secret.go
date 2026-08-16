package security

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// SecretRef is a reference to a secret, never the secret value itself. Secrets
// (passwords, tokens, private keys, device credentials) flow through the
// system as refs and are resolved only at the point of use (Design doc 10 §16).
type SecretRef string

// SecretStore is an in-memory secret provider with redaction. In production
// this is backed by a secret manager (Vault, KMS...); the reference/resolve
// contract stays identical.
type SecretStore struct {
	mu      sync.RWMutex
	secrets map[SecretRef]string
}

// NewSecretStore builds an empty SecretStore.
func NewSecretStore() *SecretStore {
	return &SecretStore{secrets: map[SecretRef]string{}}
}

// Set stores a secret value under a reference.
func (s *SecretStore) Set(ref SecretRef, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[ref] = value
}

// Resolve returns the value for a SecretRef (only at the point of use).
func (s *SecretStore) Resolve(_ context.Context, ref SecretRef) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.secrets[ref]
	if !ok {
		return "", fmt.Errorf("secret %q not found", ref)
	}
	return v, nil
}

// Redact replaces every known secret value in text with "***". Use it before
// a string enters logs, Events or Artifacts (Design doc 10 §26).
func (s *SecretStore) Redact(text string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.secrets {
		if v != "" {
			text = strings.ReplaceAll(text, v, "***")
		}
	}
	return text
}
