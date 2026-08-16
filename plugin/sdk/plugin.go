package sdk

import (
	"context"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// Plugin is the SPI every protocol implementation must satisfy. A Plugin is a
// code type; it creates Channel instances. Core owns Channel lifecycle,
// Session, Lock and Policy; the Plugin only performs protocol actions.
//
// Invariants (Design docs 04, 12):
//   - Probe must be lightweight and side-effect free.
//   - Invoke receives typed input and returns typed output; raw protocol text
//     never crosses the boundary.
//   - Errors are returned as domain.Error (normalized); raw transport text
//     goes only into Details.
//   - Recover declares what it CAN do; Core decides when it is allowed.
type Plugin interface {
	// Info returns the plugin Manifest.
	Info() Manifest

	// Probe reports whether an Endpoint belongs to this plugin.
	Probe(ctx context.Context, endpoint domain.Endpoint) (ProbeResult, error)

	// Capabilities returns the Capabilities a Channel of this plugin exposes.
	Capabilities(channel *domain.Channel) []domain.Capability

	// Open validates the Endpoint and establishes a Channel. On success the
	// channel should be READY and Healthy.
	Open(ctx context.Context, channel *domain.Channel, session domain.SessionID) error

	// Close tears down a Channel.
	Close(ctx context.Context, channel *domain.Channel) error

	// Invoke maps a unified operation to protocol actions and normalizes the
	// result.
	Invoke(ctx context.Context, channel *domain.Channel, req InvokeRequest) (*InvokeResult, error)

	// Health performs a fast, low-side-effect liveness check.
	Health(ctx context.Context, channel *domain.Channel) error

	// Observe performs a READ-ONLY observation of the device's current state
	// for a capability's postcondition (Design doc 09 §7 "Observe First"). It
	// must never mutate device state; it is used by State Reconciliation to
	// decide SUCCESS / retry / UNKNOWN / MANUAL after an interrupted operation.
	Observe(ctx context.Context, channel *domain.Channel, req InvokeRequest) (*Observation, error)

	// Recover attempts a protocol-level recovery action (e.g. reconnect). It
	// is invoked only when Core Recovery Policy permits it.
	Recover(ctx context.Context, channel *domain.Channel, reason string) error
}
