package domain

import "time"

// SessionState is the lifecycle of a Session (Design doc 07).
type SessionState string

const (
	SessionCreated      SessionState = "CREATED"
	SessionActive       SessionState = "ACTIVE"
	SessionDegraded     SessionState = "DEGRADED"
	SessionReconnecting SessionState = "RECONNECTING"
	SessionClosed       SessionState = "CLOSED"
	SessionExpired      SessionState = "EXPIRED"
	SessionRevoked      SessionState = "REVOKED"
	SessionFailed       SessionState = "FAILED"
)

// Session answers "who is accessing, in what context". It is decoupled from
// Channel: a Session can rebind to a new Channel after recovery.
type Session struct {
	ID          SessionID
	Principal   string
	DeviceID    DeviceID
	ChannelID   ChannelID
	State       SessionState
	Permissions []string
	ExpiresAt   time.Time
}

// NewSession builds a Session bound to a device for the given principal.
func NewSession(principal string, deviceID DeviceID) *Session {
	return &Session{
		ID:        NewSessionID(),
		Principal: principal,
		DeviceID:  deviceID,
		State:     SessionActive,
	}
}

// Expired reports whether the Session lease has elapsed.
func (s *Session) Expired(now time.Time) bool {
	return !s.ExpiresAt.IsZero() && now.After(s.ExpiresAt)
}

// Active reports whether the Session is usable.
func (s *Session) Active(now time.Time) bool {
	return s.State == SessionActive && !s.Expired(now)
}
