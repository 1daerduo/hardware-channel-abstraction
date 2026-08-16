// Package uart is a serial/UART console Channel Plugin. It proves the
// architecture accepts a serial transport, and it overlaps ADB on device.log
// so the Resolver demonstrates multi-channel capability selection.
//
// The plugin is transport-agnostic: it drives any sdk.ConsoleDevice, so the
// same plugin works against the fake simulator or a real serial port.
package uart

import (
	"context"
	"sync"

	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/fake"
	"example.com/embedded-loop-channel/plugin/sdk"
)

// PluginID is the stable, globally-unique plugin identity.
const PluginID = "transport.uart"

// ChannelType is the runtime channel type this plugin creates.
const ChannelType = "uart"

// Resolver maps a serial-port locator to a ConsoleDevice. The runtime
// implements it (fake farm + real serial ports).
type Resolver interface {
	BySerialPort(locator string) sdk.ConsoleDevice
}

// Plugin implements sdk.Plugin over a ConsoleDevice transport.
type Plugin struct {
	resolver Resolver

	mu       sync.RWMutex
	channels map[domain.ChannelID]sdk.ConsoleDevice
}

// New builds the UART plugin backed by a fake device farm.
func New(farm *fake.Farm) *Plugin {
	return NewWithResolver(farmResolver{farm: farm})
}

// NewWithResolver builds the UART plugin over an arbitrary device resolver
// (fake or real serial).
func NewWithResolver(r Resolver) *Plugin {
	return &Plugin{
		resolver: r,
		channels: map[domain.ChannelID]sdk.ConsoleDevice{},
	}
}

// Info returns the plugin Manifest.
func (p *Plugin) Info() sdk.Manifest {
	return sdk.Manifest{
		ID:              PluginID,
		Version:         "1.0.0",
		APIVersion:      "1.0",
		Protocol:        "uart",
		Capabilities:    capabilityNames(),
		Transports:      []string{"serial"},
		RecoveryActions: []string{"reopen", "reconnect"},
		TrustLevel:      sdk.TrustVerified,
	}
}

// Probe reports whether an endpoint is a serial port owned by this plugin.
func (p *Plugin) Probe(_ context.Context, endpoint domain.Endpoint) (sdk.ProbeResult, error) {
	if endpoint.Type != domain.EndpointUART {
		return sdk.ProbeResult{Match: false}, nil
	}
	dev := p.resolver.BySerialPort(endpoint.Locator)
	if dev == nil {
		return sdk.ProbeResult{Match: false}, nil
	}
	return sdk.ProbeResult{
		Match:         true,
		Confidence:    1.0,
		IdentityHints: dev.Identity(),
		ChannelType:   ChannelType,
		Cost:          5, // cheaper than ADB, so it wins device.log resolution
	}, nil
}

// Capabilities returns the UART capability set.
func (p *Plugin) Capabilities(*domain.Channel) []domain.Capability {
	return capabilityDescriptors()
}

// Open binds the channel to the console device behind its serial port.
func (p *Plugin) Open(_ context.Context, channel *domain.Channel, _ domain.SessionID) error {
	return p.bind(channel)
}

// Close releases the channel binding.
func (p *Plugin) Close(_ context.Context, channel *domain.Channel) error {
	p.mu.Lock()
	delete(p.channels, channel.ID)
	p.mu.Unlock()
	channel.State = domain.ChannelClosed
	channel.Healthy = false
	return nil
}

// Health performs a fast liveness check.
func (p *Plugin) Health(_ context.Context, channel *domain.Channel) error {
	dev := p.device(channel.ID)
	if dev == nil {
		return sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}
	if !dev.IsOnline() {
		channel.Healthy = false
		return sdk.DeviceStateError(domain.CodeDeviceOffline, "device offline")
	}
	channel.Healthy = true
	return nil
}

// Cancel accepts a cooperative-cancellation request. A real serial command
// cannot be interrupted mid-transfer; the engine observes the cancel flag
// between lifecycle steps instead.
func (p *Plugin) Cancel(_ context.Context, _ *domain.Channel, _ domain.OperationID) error {
	return nil
}

// Recover re-binds a lost channel. reason "device_recovery" additionally
// power-cycles the device (the L5 high-risk recovery action).
func (p *Plugin) Recover(_ context.Context, channel *domain.Channel, reason string) error {
	channel.State = domain.ChannelReconnecting
	if reason == "device_recovery" {
		if dev := p.resolver.BySerialPort(channel.Locator); dev != nil {
			_ = dev.PowerCycle()
		}
	}
	return p.bind(channel)
}

func (p *Plugin) bind(channel *domain.Channel) error {
	dev := p.resolver.BySerialPort(channel.Locator)
	if dev == nil {
		return sdk.ConnectionError(domain.CodeDeviceOffline, "no device at serial port "+channel.Locator)
	}
	if !dev.IsOnline() {
		return sdk.DeviceStateError(domain.CodeDeviceOffline, "device is offline")
	}
	p.mu.Lock()
	p.channels[channel.ID] = dev
	p.mu.Unlock()
	channel.State = domain.ChannelReady
	channel.Healthy = true
	return nil
}

func (p *Plugin) device(id domain.ChannelID) sdk.ConsoleDevice {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.channels[id]
}

// farmResolver adapts a fake device farm to the Resolver interface.
type farmResolver struct{ farm *fake.Farm }

func (r farmResolver) BySerialPort(locator string) sdk.ConsoleDevice {
	d := r.farm.BySerialPort(locator)
	if d == nil {
		return nil
	}
	return d
}
