// Package session manages Session lifecycle (Design doc 07). A Session is the
// access context; it is decoupled from Channel and can rebind after recovery.
package session

import (
	"fmt"
	"sync"
	"time"

	"example.com/embedded-loop-channel/domain"
)

// Manager is a thread-safe Session store.
type Manager struct {
	mu       sync.RWMutex
	sessions map[domain.SessionID]*domain.Session
}

// NewManager builds an empty Manager.
func NewManager() *Manager {
	return &Manager{sessions: map[domain.SessionID]*domain.Session{}}
}

// Create opens a Session for principal bound to device, expiring after ttl
// (ttl <= 0 means no expiry).
func (m *Manager) Create(principal string, deviceID domain.DeviceID, ttl time.Duration) (*domain.Session, error) {
	s := domain.NewSession(principal, deviceID)
	if ttl > 0 {
		s.ExpiresAt = time.Now().Add(ttl)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
	return s, nil
}

// Get returns a Session by ID.
func (m *Manager) Get(id domain.SessionID) (*domain.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session: %s not found", id)
	}
	return s, nil
}

// Renew extends the session lease; expired sessions cannot be renewed.
func (m *Manager) Renew(id domain.SessionID, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session: %s not found", id)
	}
	if s.State == domain.SessionClosed || s.State == domain.SessionRevoked {
		return fmt.Errorf("session: %s is %s", id, s.State)
	}
	if s.Expired(time.Now()) {
		s.State = domain.SessionExpired
		return fmt.Errorf("session: %s expired", id)
	}
	s.ExpiresAt = time.Now().Add(ttl)
	return nil
}

// Close terminates the session.
func (m *Manager) Close(id domain.SessionID) error {
	return m.setState(id, domain.SessionClosed)
}

// Revoke force-terminates the session (administrative action).
func (m *Manager) Revoke(id domain.SessionID) error {
	return m.setState(id, domain.SessionRevoked)
}

// BindChannel records the channel a session is currently using.
func (m *Manager) BindChannel(id domain.SessionID, ch domain.ChannelID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session: %s not found", id)
	}
	s.ChannelID = ch
	return nil
}

func (m *Manager) setState(id domain.SessionID, st domain.SessionState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session: %s not found", id)
	}
	s.State = st
	return nil
}
