package recovery

import (
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// Level is the recovery ladder (Design doc 09 §6). Levels escalate: a lower
// level failing falls through to the next.
type Level int

const (
	L0Observe            Level = iota // Observe-first: read device state
	L1Retry                           // Retry the operation (engine-owned)
	L2ReconnectChannel                // Repair the original Channel
	L3ReResolveChannel                // Pick another Channel
	L4RediscoverDevice                // Rebuild Endpoint/Device/Channel
	L5DeviceRecovery                  // Reset / power-cycle (high risk)
	L6ManualIntervention              // Human required
)

func (l Level) String() string {
	switch l {
	case L0Observe:
		return "OBSERVE"
	case L1Retry:
		return "RETRY"
	case L2ReconnectChannel:
		return "RECONNECT"
	case L3ReResolveChannel:
		return "RE_RESOLVE"
	case L4RediscoverDevice:
		return "REDISCOVER"
	case L5DeviceRecovery:
		return "DEVICE_RECOVERY"
	case L6ManualIntervention:
		return "MANUAL"
	}
	return "UNKNOWN"
}

// State is the recovery state machine (Design doc 09 §16).
type State string

const (
	StateNormal         State = "NORMAL"
	StateDegraded       State = "DEGRADED"
	StateRecovering     State = "RECOVERING"
	StateObserving      State = "OBSERVING"
	StateReconnecting   State = "RECONNECTING"
	StateReResolving    State = "RE_RESOLVING"
	StateRediscovering  State = "REDISCOVERING"
	StateDeviceRecovery State = "DEVICE_RECOVERY"
	StateRecovered      State = "RECOVERED"
	StateRecoveryFailed State = "RECOVERY_FAILED"
	StateUnknown        State = "UNKNOWN"
	StateManual         State = "MANUAL"
)

// Budget bounds recovery so a device farm never enters a Reset Storm
// (Design doc 09 §17).
type Budget struct {
	MaxAttempts     int
	MaxDuration     time.Duration
	MaxDeviceResets int
	MaxPowerCycles  int
	BaseBackoff     time.Duration
}

// DefaultBudget returns a conservative MVP budget.
func DefaultBudget() Budget {
	return Budget{
		MaxAttempts:     3,
		MaxDuration:     30 * time.Second,
		MaxDeviceResets: 1,
		MaxPowerCycles:  0,
		BaseBackoff:     10 * time.Millisecond,
	}
}

// RecoveryResult is the outcome of a recovery attempt. Channel is the (possibly
// new) channel to use after recovery; L3 re-resolve and L4 rediscover may
// select a different channel than the failed one.
type RecoveryResult struct {
	State     State
	Level     Level
	Recovered bool
	Channel   *domain.Channel
	Error     *domain.Error
}
