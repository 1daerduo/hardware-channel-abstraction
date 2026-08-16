package sdk

import (
	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// ProbeResult is the outcome of a Probe. Match reports ownership, confidence
// ranks competing plugins, and IdentityHints feed Device Correlation.
type ProbeResult struct {
	Match         bool
	Confidence    float64
	IdentityHints map[string]string
	ChannelType   string
	Cost          int64
}

// InvokeRequest is the typed input to Plugin.Invoke. Upper layers express a
// capability + parameters; the plugin translates it into protocol actions.
type InvokeRequest struct {
	Capability  domain.CapabilityName
	Target      domain.DeviceID
	Parameters  map[string]string
	SessionID   domain.SessionID
	OperationID domain.OperationID
}

// InvokeResult is the typed, normalized output of Plugin.Invoke. Evidence and
// Artifacts ride alongside the result; raw protocol text is already mapped
// away.
type InvokeResult struct {
	Output    string
	Evidence  []domain.Evidence
	Artifacts []domain.Artifact
}

// Observation is the read-only snapshot returned by Plugin.Observe. It is
// used for Observe-first reconciliation after an interrupted/UNKNOWN
// operation. Facts are postcondition facts (e.g. "flash.version" -> "2.0.0").
type Observation struct {
	Online bool
	State  string
	Facts  map[string]string
}
