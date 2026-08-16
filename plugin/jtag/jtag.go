// Package jtag is a reference Channel Plugin for the JTAG/SWD debug control
// plane. It proves the architecture accepts a debugger: exclusive debug
// resources, high-risk halt/flash/memory actions, all through the SAME Plugin
// SPI as ADB/UART/TCP/MCP.
package jtag

import (
	"context"
	"sync"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// PluginID is the stable, globally-unique plugin identity.
const PluginID = "protocol.jtag"

// ChannelType is the runtime channel type this plugin creates.
const ChannelType = "jtag"

// Resolver maps a JTAG locator to a fake debug target.
type Resolver interface {
	ByJTAGLocator(locator string) *fake.Device
}

// Plugin implements sdk.Plugin over the fake debug target.
type Plugin struct {
	resolver Resolver

	mu       sync.RWMutex
	channels map[domain.ChannelID]*fake.Device
}

// NewWithResolver builds the JTAG plugin.
func NewWithResolver(r Resolver) *Plugin {
	return &Plugin{
		resolver: r,
		channels: map[domain.ChannelID]*fake.Device{},
	}
}

// Info returns the plugin Manifest.
func (p *Plugin) Info() sdk.Manifest {
	return sdk.Manifest{
		ID:              PluginID,
		Version:         "1.0.0",
		APIVersion:      "1.0",
		Protocol:        "jtag",
		Capabilities:    capabilityNames(),
		Transports:      []string{"debug-probe"},
		RecoveryActions: []string{"reconnect", "reset-debug-session"},
		TrustLevel:      sdk.TrustVerified,
	}
}

// Probe reports whether an endpoint is a JTAG debug target owned by this
// plugin.
func (p *Plugin) Probe(_ context.Context, endpoint domain.Endpoint) (sdk.ProbeResult, error) {
	if endpoint.Type != domain.EndpointJTAG {
		return sdk.ProbeResult{Match: false}, nil
	}
	dev := p.resolver.ByJTAGLocator(endpoint.Locator)
	if dev == nil {
		return sdk.ProbeResult{Match: false}, nil
	}
	return sdk.ProbeResult{
		Match:         true,
		Confidence:    1.0,
		IdentityHints: dev.Identity(),
		ChannelType:   ChannelType,
		Cost:          30, // debug is exclusive/high-cost
	}, nil
}

// Capabilities returns the JTAG debug capability set.
func (p *Plugin) Capabilities(*domain.Channel) []domain.Capability {
	return capabilityDescriptors()
}

// Open binds the channel to the fake debug target.
func (p *Plugin) Open(_ context.Context, channel *domain.Channel, _ domain.SessionID) error {
	dev := p.resolver.ByJTAGLocator(channel.Locator)
	if dev == nil {
		return sdk.ConnectionError(domain.CodeDeviceOffline, "no debug target at "+channel.Locator)
	}
	p.mu.Lock()
	p.channels[channel.ID] = dev
	p.mu.Unlock()
	channel.State = domain.ChannelReady
	channel.Healthy = true
	return nil
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

// Recover re-binds the channel.
func (p *Plugin) Recover(_ context.Context, channel *domain.Channel, _ string) error {
	channel.State = domain.ChannelReconnecting
	dev := p.resolver.ByJTAGLocator(channel.Locator)
	if dev == nil {
		return sdk.ConnectionError(domain.CodeDeviceOffline, "no debug target")
	}
	p.mu.Lock()
	p.channels[channel.ID] = dev
	p.mu.Unlock()
	channel.State = domain.ChannelReady
	channel.Healthy = true
	return nil
}

// Cancel accepts a cooperative-cancellation request.
func (p *Plugin) Cancel(_ context.Context, _ *domain.Channel, _ domain.OperationID) error {
	return nil
}

func (p *Plugin) device(id domain.ChannelID) *fake.Device {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.channels[id]
}
