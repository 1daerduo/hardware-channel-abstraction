// Package recovery implements Error Classification and a minimal Recovery
// Manager (Design doc 09). The guiding rule is "Observe first": a lost result
// is UNKNOWN, and recovery is governed by Policy and Budget, never by a blind
// while-loop retry.
package recovery

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/core/discovery"
	"github.com/1daerduo/hardware-channel-abstraction/core/event"
	"github.com/1daerduo/hardware-channel-abstraction/core/resolver"
	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/registry"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// Classifier maps arbitrary errors to the unified taxonomy. Already-normalized
// errors pass through unchanged; automation depends on code/category only.
type Classifier struct{}

// NewClassifier builds a Classifier.
func NewClassifier() *Classifier { return &Classifier{} }

// Classify normalizes err into a *domain.Error.
func (c *Classifier) Classify(err error) *domain.Error {
	if err == nil {
		return nil
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return de
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return domain.NewError(domain.CodeTimeout, domain.CategoryTimeout, "deadline exceeded").
			WithDetail("retryable", "false")
	case errors.Is(err, context.Canceled):
		return domain.NewError(domain.CodeUnknown, domain.CategoryCancellation, "cancelled")
	default:
		return domain.NewError(domain.CodeUnknown, domain.CategoryUnknown, err.Error())
	}
}

// attemptState tracks per-channel recovery budget consumption.
type attemptState struct {
	attempts     int
	deviceResets int
	lastBackoff  time.Duration
}

// Manager executes the recovery ladder (Design doc 09 §6) subject to a
// Budget. It owns protocol-level actions (Observe, Reconnect, Re-resolve,
// Rediscover, Device Recovery) and escalates L2 → L3 → L4 → L5 → L6; the
// Operation Engine owns L1 (retry) and feeds the recovered channel back into
// the invoke loop.
type Manager struct {
	plugins   *registry.Registry
	bus       *event.Bus
	resolver  *resolver.Resolver
	discovery *discovery.Service
	budget    Budget
	now       func() time.Time

	mu       sync.Mutex
	attempts map[domain.ChannelID]*attemptState
}

// NewManager builds a recovery Manager with the given budget.
func NewManager(plugins *registry.Registry, bus *event.Bus, budget Budget) *Manager {
	if budget.MaxAttempts <= 0 {
		budget.MaxAttempts = 3
	}
	if budget.BaseBackoff <= 0 {
		budget.BaseBackoff = 10 * time.Millisecond
	}
	return &Manager{
		plugins:  plugins,
		bus:      bus,
		budget:   budget,
		now:      time.Now,
		attempts: map[domain.ChannelID]*attemptState{},
	}
}

// WithResolver wires the Channel Resolver for L3 re-resolve.
func (m *Manager) WithResolver(r *resolver.Resolver) *Manager {
	m.resolver = r
	return m
}

// WithDiscovery wires the Discovery Service for L4 rediscover.
func (m *Manager) WithDiscovery(d *discovery.Service) *Manager {
	m.discovery = d
	return m
}

// Observe runs L0 (observe-first) via the plugin, returning a read-only
// snapshot. It never mutates device state.
func (m *Manager) Observe(ctx context.Context, channel *domain.Channel, req sdk.InvokeRequest) (*sdk.Observation, error) {
	plugin, err := m.plugins.Get(channel.PluginID)
	if err != nil {
		return nil, domain.NewError(domain.CodeInternal, domain.CategoryInternal, err.Error())
	}
	return plugin.Observe(ctx, channel, req)
}

// Recover runs the recovery ladder for a failed operation on channel,
// returning the final RecoveryResult. It escalates L2 reconnect → L3
// re-resolve → L4 rediscover → L5 device recovery → L6 manual, bounded by the
// Budget, with exponential backoff.
func (m *Manager) Recover(ctx context.Context, channel *domain.Channel, capability domain.CapabilityName, derr *domain.Error) RecoveryResult {
	if derr == nil {
		derr = domain.NewError(domain.CodeUnknown, domain.CategoryUnknown, "unknown failure")
	}
	st := m.stateFor(channel.ID)
	if st.attempts >= m.budget.MaxAttempts {
		return RecoveryResult{State: StateRecoveryFailed, Level: L6ManualIntervention, Error: derr, Channel: channel}
	}

	m.emitRecoveryStarted(channel, derr)

	// L2: reconnect the original channel.
	if m.canReconnect(derr) {
		m.emitStep(channel, L2ReconnectChannel)
		m.backoff(st)
		if m.recoverVia(channel, "reconnect", ctx) == nil {
			m.reset(channel.ID)
			m.emitRecoveryCompleted(channel)
			return RecoveryResult{State: StateRecovered, Level: L2ReconnectChannel, Recovered: true, Channel: channel}
		}
	}

	// L3: re-resolve to a different channel.
	if m.resolver != nil && m.canReResolve(derr) {
		m.emitStep(channel, L3ReResolveChannel)
		m.backoff(st)
		if alt := m.reResolve(channel, capability); alt != nil {
			m.reset(channel.ID)
			m.emitRecoveryCompleted(alt)
			return RecoveryResult{State: StateRecovered, Level: L3ReResolveChannel, Recovered: true, Channel: alt}
		}
	}

	// L4: rediscover endpoints/devices/channels, then re-resolve.
	if m.discovery != nil && m.canRediscover(derr) {
		m.emitStep(channel, L4RediscoverDevice)
		m.backoff(st)
		if _, err := m.discovery.Discover(ctx); err == nil {
			if alt := m.reResolve(channel, capability); alt != nil {
				m.reset(channel.ID)
				m.emitRecoveryCompleted(alt)
				return RecoveryResult{State: StateRecovered, Level: L4RediscoverDevice, Recovered: true, Channel: alt}
			}
		}
	}

	// L5: device recovery (power cycle), high risk and budget-capped.
	if m.canDeviceRecover(derr) && st.deviceResets < m.budget.MaxDeviceResets {
		st.deviceResets++
		m.emitStep(channel, L5DeviceRecovery)
		m.backoff(st)
		if m.recoverVia(channel, "device_recovery", ctx) == nil {
			m.reset(channel.ID)
			m.emitRecoveryCompleted(channel)
			return RecoveryResult{State: StateRecovered, Level: L5DeviceRecovery, Recovered: true, Channel: channel}
		}
	}

	// L6: manual intervention.
	m.emitRecoveryExhausted(channel)
	return RecoveryResult{State: StateManual, Level: L6ManualIntervention, Error: derr, Channel: channel}
}

// reResolve marks the failed channel degraded and picks another READY channel
// for the capability via the Resolver.
func (m *Manager) reResolve(failed *domain.Channel, capability domain.CapabilityName) *domain.Channel {
	if failed.State == domain.ChannelReady {
		failed.State = domain.ChannelDegraded
		failed.Healthy = false
	}
	res, err := m.resolver.ResolveChannel(failed.DeviceID, capability, "")
	if err != nil {
		return nil
	}
	if res.Channel.ID == failed.ID {
		return nil // no alternative channel
	}
	return res.Channel
}

// canReconnect reports whether L2 applies to the failure.
func (m *Manager) canReconnect(derr *domain.Error) bool {
	return derr.Recoverable || derr.Category == domain.CategoryConnection ||
		derr.Category == domain.CategoryDeviceState
}

// canDeviceRecover reports whether L5 (power cycle) is permitted for the
// failure. Power cycling is a last resort for device/connection failures only.
func (m *Manager) canDeviceRecover(derr *domain.Error) bool {
	return derr.Category == domain.CategoryDeviceState || derr.Category == domain.CategoryConnection
}

// canReResolve reports whether L3 applies: there may be another channel for
// the capability.
func (m *Manager) canReResolve(derr *domain.Error) bool {
	return derr.Recoverable || derr.Category == domain.CategoryConnection ||
		derr.Category == domain.CategoryDeviceState || derr.Category == domain.CategoryProtocol
}

// canRediscover reports whether L4 applies: re-scan sources to rebuild
// Endpoint/Device/Channel knowledge.
func (m *Manager) canRediscover(derr *domain.Error) bool {
	return derr.Category == domain.CategoryConnection ||
		derr.Category == domain.CategoryDeviceState ||
		derr.Category == domain.CategoryDiscovery
}

func (m *Manager) recoverVia(channel *domain.Channel, reason string, ctx context.Context) error {
	plugin, err := m.plugins.Get(channel.PluginID)
	if err != nil {
		return err
	}
	return plugin.Recover(ctx, channel, reason)
}

func (m *Manager) backoff(st *attemptState) {
	st.attempts++
	if st.attempts <= 1 {
		return
	}
	d := m.budget.BaseBackoff << (st.attempts - 1)
	if d > time.Second {
		d = time.Second
	}
	st.lastBackoff = d
	time.Sleep(d)
}

func (m *Manager) stateFor(id domain.ChannelID) *attemptState {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.attempts[id]
	if !ok {
		st = &attemptState{}
		m.attempts[id] = st
	}
	return st
}

func (m *Manager) reset(id domain.ChannelID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.attempts, id)
}

// ---------------------------------------------------------------------------
// Recovery events (Design doc 09 §20)
// ---------------------------------------------------------------------------

func (m *Manager) emitRecoveryStarted(channel *domain.Channel, derr *domain.Error) {
	e := domain.NewEvent(domain.EventRecoveryStarted, "core.recovery", "channel")
	e.WithChannel(channel.ID).WithDevice(channel.DeviceID)
	e.Payload = map[string]string{"code": derr.Code, "category": string(derr.Category)}
	m.bus.Publish(e)
}

func (m *Manager) emitStep(channel *domain.Channel, level Level) {
	e := domain.NewEvent("RecoveryStepStarted", "core.recovery", "channel")
	e.WithChannel(channel.ID).WithDevice(channel.DeviceID)
	e.Payload = map[string]string{"level": level.String()}
	m.bus.Publish(e)
}

func (m *Manager) emitRecoveryCompleted(channel *domain.Channel) {
	e := domain.NewEvent(domain.EventRecoveryCompleted, "core.recovery", "channel")
	e.WithChannel(channel.ID).WithDevice(channel.DeviceID)
	m.bus.Publish(e)
}

func (m *Manager) emitRecoveryExhausted(channel *domain.Channel) {
	e := domain.NewEvent("RecoveryExhausted", "core.recovery", "channel")
	e.WithChannel(channel.ID).WithDevice(channel.DeviceID)
	m.bus.Publish(e)
}
