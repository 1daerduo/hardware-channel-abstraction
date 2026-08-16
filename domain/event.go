package domain

import "time"

// Event is a fact ("what happened"), distinct from Result (the conclusion)
// and Evidence (why we believe it).
type Event struct {
	ID          EventID
	Type        string
	Version     string
	Producer    string
	Subject     string
	Sequence    uint64
	OccurredAt  time.Time
	EmittedAt   time.Time
	OperationID OperationID
	DeviceID    DeviceID
	ChannelID   ChannelID
	SessionID   SessionID
	Payload     map[string]string
}

// NewEvent builds an Event with a fresh ID and both timestamps set.
func NewEvent(typ, producer, subject string) *Event {
	now := time.Now()
	return &Event{
		ID:         NewEventID(),
		Type:       typ,
		Version:    "1.0",
		Producer:   producer,
		Subject:    subject,
		OccurredAt: now,
		EmittedAt:  now,
		Payload:    map[string]string{},
	}
}

// WithOperation attaches operation correlation.
func (e *Event) WithOperation(id OperationID) *Event {
	e.OperationID = id
	return e
}

// WithDevice attaches device correlation.
func (e *Event) WithDevice(id DeviceID) *Event {
	e.DeviceID = id
	return e
}

// WithChannel attaches channel correlation.
func (e *Event) WithChannel(id ChannelID) *Event {
	e.ChannelID = id
	return e
}

// Standard event types (facts, not commands).
const (
	EventDeviceOnline          = "DeviceOnline"
	EventDeviceOffline         = "DeviceOffline"
	EventChannelReady          = "ChannelReady"
	EventChannelLost           = "ChannelLost"
	EventOperationStarted      = "OperationStarted"
	EventOperationProgress     = "OperationProgress"
	EventOperationSucceeded    = "OperationSucceeded"
	EventOperationFailed       = "OperationFailed"
	EventOperationStateUnknown = "OperationStateUnknown"
	EventRecoveryStarted       = "RecoveryStarted"
	EventRecoveryCompleted     = "RecoveryCompleted"
	EventEndpointUnavailable   = "EndpointUnavailable"
	EventIdentityConflict      = "IdentityConflict"
)
