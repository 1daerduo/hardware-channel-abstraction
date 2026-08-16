// Package tcp is a reference Channel Plugin for a TCP-connected console
// device. It proves the architecture accepts a network transport with zero
// Core change: the plugin drives any sdk.ConsoleDevice, and the TCP transport
// is a thin wrapper over the shared byte-stream console.
package tcp

import (
	"context"
	"sync"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// PluginID is the stable, globally-unique plugin identity.
const PluginID = "protocol.tcp"

// ChannelType is the runtime channel type this plugin creates.
const ChannelType = "tcp"

// Resolver maps a TCP address to a ConsoleDevice.
type Resolver interface {
	ByTCPAddr(addr string) sdk.ConsoleDevice
}

// Plugin implements sdk.Plugin over a TCP console transport.
type Plugin struct {
	resolver Resolver

	mu       sync.RWMutex
	channels map[domain.ChannelID]sdk.ConsoleDevice
}

// NewWithResolver builds the TCP plugin over a device resolver.
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
		Protocol:        "tcp",
		Capabilities:    capabilityNames(),
		Transports:      []string{"tcp-ip"},
		RecoveryActions: []string{"reconnect"},
		TrustLevel:      sdk.TrustVerified,
	}
}

// Probe reports whether an endpoint is a TCP console owned by this plugin.
func (p *Plugin) Probe(_ context.Context, endpoint domain.Endpoint) (sdk.ProbeResult, error) {
	if endpoint.Type != domain.EndpointTCP {
		return sdk.ProbeResult{Match: false}, nil
	}
	dev := p.resolver.ByTCPAddr(endpoint.Locator)
	if dev == nil {
		return sdk.ProbeResult{Match: false}, nil
	}
	return sdk.ProbeResult{
		Match:         true,
		Confidence:    1.0,
		IdentityHints: dev.Identity(),
		ChannelType:   ChannelType,
		Cost:          15,
	}, nil
}

// Capabilities returns the TCP console capability set.
func (p *Plugin) Capabilities(*domain.Channel) []domain.Capability {
	return capabilityDescriptors()
}

// Open binds the channel to the console device behind its TCP address.
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

// Recover re-binds a lost channel.
func (p *Plugin) Recover(_ context.Context, channel *domain.Channel, _ string) error {
	channel.State = domain.ChannelReconnecting
	return p.bind(channel)
}

// Cancel accepts a cooperative-cancellation request.
func (p *Plugin) Cancel(_ context.Context, _ *domain.Channel, _ domain.OperationID) error {
	return nil
}

func (p *Plugin) bind(channel *domain.Channel) error {
	dev := p.resolver.ByTCPAddr(channel.Locator)
	if dev == nil {
		return sdk.ConnectionError(domain.CodeDeviceOffline, "no device at tcp "+channel.Locator)
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
