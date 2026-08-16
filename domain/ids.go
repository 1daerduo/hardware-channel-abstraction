// Package domain defines the stable core concepts of the Embedded Loop
// Channel Abstraction: Device, Endpoint, Channel, Capability, Operation,
// Session, Resource, Error, Event and Artifact.
//
// Domain does not depend on Core or on any concrete Plugin. Every runtime
// object carries a stable string ID; an ID never encodes a physical address.
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
)

// Typed string identifiers. They are stable, opaque, and correlation-safe.
type (
	DeviceID       string
	EndpointID     string
	ChannelID      string
	CapabilityName string
	OperationID    string
	SessionID      string
	ResourceID     string
	LeaseID        string
	EventID        string
	ArtifactID     string
	EvidenceID     string
)

var idCounter atomic.Uint64

// NewID returns a unique, opaque ID prefixed with p. It is suitable for all
// runtime objects and never embeds a physical address or a counter that
// consumers may depend on.
func NewID(p string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is extraordinarily rare; fall back to a
		// monotonic counter so callers never observe an empty ID.
		return p + "-" + hex.EncodeToString([]byte{byte(idCounter.Add(1))})
	}
	return p + "-" + hex.EncodeToString(b[:])
}

// NewDeviceID returns a fresh Device ID.
func NewDeviceID() DeviceID { return DeviceID(NewID("dev")) }

// NewEndpointID returns a fresh Endpoint ID.
func NewEndpointID() EndpointID { return EndpointID(NewID("ep")) }

// NewChannelID returns a fresh Channel ID.
func NewChannelID() ChannelID { return ChannelID(NewID("ch")) }

// NewOperationID returns a fresh Operation ID.
func NewOperationID() OperationID { return OperationID(NewID("op")) }

// NewSessionID returns a fresh Session ID.
func NewSessionID() SessionID { return SessionID(NewID("sess")) }

// NewResourceID returns a fresh Resource ID.
func NewResourceID() ResourceID { return ResourceID(NewID("res")) }

// NewLeaseID returns a fresh Lease ID.
func NewLeaseID() LeaseID { return LeaseID(NewID("lease")) }

// NewEventID returns a fresh Event ID.
func NewEventID() EventID { return EventID(NewID("evt")) }

// NewArtifactID returns a fresh Artifact ID.
func NewArtifactID() ArtifactID { return ArtifactID(NewID("art")) }

// NewEvidenceID returns a fresh Evidence ID.
func NewEvidenceID() EvidenceID { return EvidenceID(NewID("evid")) }
