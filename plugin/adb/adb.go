// Package adb is the reference ADB-like Channel Plugin. It talks to fake
// devices through an in-memory Farm (the stand-in for an `adb server`). It
// demonstrates how a concrete protocol maps onto the unified Plugin SPI,
// Capability, Operation, Error and Evidence contracts without any of it
// leaking into Core.
package adb

import (
	"context"
	"sync"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// PluginID is the stable, globally-unique plugin identity (Design doc 12 §5).
const PluginID = "protocol.adb"

// ChannelType is the runtime channel type this plugin creates.
const ChannelType = "adb"

// Plugin implements sdk.Plugin against an in-memory fake device farm.
type Plugin struct {
	farm *fake.Farm

	mu       sync.RWMutex
	channels map[domain.ChannelID]*fake.Device
}

// New builds the ADB plugin backed by farm.
func New(farm *fake.Farm) *Plugin {
	return &Plugin{
		farm:     farm,
		channels: map[domain.ChannelID]*fake.Device{},
	}
}

// Info returns the plugin Manifest.
func (p *Plugin) Info() sdk.Manifest {
	return sdk.Manifest{
		ID:              PluginID,
		Version:         "1.0.0",
		APIVersion:      "1.0",
		Protocol:        "adb",
		Capabilities:    capabilityNames(),
		Transports:      []string{"usb", "tcp-ip"},
		RecoveryActions: []string{"reconnect", "rediscover"},
		TrustLevel:      sdk.TrustVerified,
	}
}

// Probe reports whether an endpoint is an ADB device owned by this plugin.
// It is side-effect free and resolves the locator against the farm.
func (p *Plugin) Probe(_ context.Context, endpoint domain.Endpoint) (sdk.ProbeResult, error) {
	if endpoint.Type != domain.EndpointUSBADB {
		return sdk.ProbeResult{Match: false}, nil
	}
	dev := p.farm.ByLocator(endpoint.Locator)
	if dev == nil {
		return sdk.ProbeResult{Match: false}, nil
	}
	return sdk.ProbeResult{
		Match:         true,
		Confidence:    1.0,
		IdentityHints: dev.Identity(),
		ChannelType:   ChannelType,
		Cost:          10,
	}, nil
}

// Capabilities returns the standard ADB capability set.
func (p *Plugin) Capabilities(*domain.Channel) []domain.Capability {
	return capabilityDescriptors()
}

// Open binds the channel to the fake device behind its endpoint locator.
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

// Cancel accepts a cooperative-cancellation request. Fake-device invokes are
// instant, so there is nothing to interrupt; the engine observes the cancel
// flag between lifecycle steps.
func (p *Plugin) Cancel(_ context.Context, _ *domain.Channel, _ domain.OperationID) error {
	return nil
}

// Recover re-binds a lost channel. reason "device_recovery" additionally
// power-cycles the device (the L5 high-risk recovery action).
func (p *Plugin) Recover(_ context.Context, channel *domain.Channel, reason string) error {
	channel.State = domain.ChannelReconnecting
	if reason == "device_recovery" {
		if dev := p.farm.ByLocator(channel.Locator); dev != nil {
			_ = dev.PowerCycle()
		}
	}
	return p.bind(channel)
}

// bind resolves a channel to its fake device via the channel's endpoint
// locator (recorded by Discovery) and marks it READY.
func (p *Plugin) bind(channel *domain.Channel) error {
	dev := p.farm.ByLocator(channel.Locator)
	if dev == nil {
		return sdk.ConnectionError(domain.CodeDeviceOffline, "no fake device at locator "+channel.Locator)
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
