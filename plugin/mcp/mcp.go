// Package mcp is the third reference Channel Plugin: a Model Context Protocol
// (MCP) remote-service adapter. It proves the architecture accepts a
// network/remote protocol alongside USB-ADB and serial/UART with zero Core
// change, and it overlaps ADB on device.info.get / device.reboot so the
// Resolver ranks three protocols deterministically.
package mcp

import (
	"context"
	"sync"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// PluginID is the stable, globally-unique plugin identity.
const PluginID = "protocol.mcp"

// ChannelType is the runtime channel type this plugin creates.
const ChannelType = "mcp"

// Plugin implements sdk.Plugin over a fake MCP remote service.
type Plugin struct {
	farm *fake.Farm

	mu       sync.RWMutex
	channels map[domain.ChannelID]*fake.Device
}

// New builds the MCP plugin backed by farm.
func New(farm *fake.Farm) *Plugin {
	return &Plugin{
		farm:     farm,
		channels: map[domain.ChannelID]*fake.Device{},
	}
}

// Info returns the plugin Manifest. MCP exposes tools mapped to unified
// capabilities.
func (p *Plugin) Info() sdk.Manifest {
	return sdk.Manifest{
		ID:              PluginID,
		Version:         "1.0.0",
		APIVersion:      "1.0",
		Protocol:        "mcp",
		Capabilities:    capabilityNames(),
		Transports:      []string{"http", "stdio"},
		RecoveryActions: []string{"reconnect", "rediscover"},
		TrustLevel:      sdk.TrustVerified,
	}
}

// Probe reports whether an endpoint is an MCP remote service owned by this
// plugin.
func (p *Plugin) Probe(_ context.Context, endpoint domain.Endpoint) (sdk.ProbeResult, error) {
	if endpoint.Type != domain.EndpointMCP {
		return sdk.ProbeResult{Match: false}, nil
	}
	dev := p.farm.ByMCPURL(endpoint.Locator)
	if dev == nil {
		return sdk.ProbeResult{Match: false}, nil
	}
	return sdk.ProbeResult{
		Match:         true,
		Confidence:    1.0,
		IdentityHints: dev.Identity(),
		ChannelType:   ChannelType,
		Cost:          20, // highest cost: loses to ADB/UART unless overridden
	}, nil
}

// Capabilities returns the MCP capability set (mapped from tools).
func (p *Plugin) Capabilities(*domain.Channel) []domain.Capability {
	return capabilityDescriptors()
}

// Open binds the channel to the fake device behind its MCP endpoint.
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

// Cancel accepts a cooperative-cancellation request.
func (p *Plugin) Cancel(_ context.Context, _ *domain.Channel, _ domain.OperationID) error {
	return nil
}

// Recover re-binds a lost channel; "device_recovery" power-cycles.
func (p *Plugin) Recover(_ context.Context, channel *domain.Channel, reason string) error {
	channel.State = domain.ChannelReconnecting
	if reason == "device_recovery" {
		if dev := p.farm.ByMCPURL(channel.Locator); dev != nil {
			_ = dev.PowerCycle()
		}
	}
	return p.bind(channel)
}

func (p *Plugin) bind(channel *domain.Channel) error {
	dev := p.farm.ByMCPURL(channel.Locator)
	if dev == nil {
		return sdk.ConnectionError(domain.CodeDeviceOffline, "no fake device at mcp endpoint "+channel.Locator)
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

func (p *Plugin) device(id domain.ChannelID) *fake.Device {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.channels[id]
}
