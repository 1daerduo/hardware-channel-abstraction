// Package modbus is a reference Channel Plugin for a Modbus TCP device. It
// proves the architecture accepts a STRUCTURED (non-byte-stream) industrial
// register protocol: the plugin speaks the Modbus framing itself and maps it
// to typed capabilities, while Core stays protocol-agnostic.
package modbus

import (
	"context"
	"sync"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// PluginID is the stable, globally-unique plugin identity.
const PluginID = "protocol.modbus"

// ChannelType is the runtime channel type this plugin creates.
const ChannelType = "modbus"

// Plugin implements sdk.Plugin over a Modbus TCP connection.
type Plugin struct {
	mu       sync.RWMutex
	channels map[domain.ChannelID]*client
}

// New builds the Modbus plugin.
func New() *Plugin {
	return &Plugin{channels: map[domain.ChannelID]*client{}}
}

// Info returns the plugin Manifest.
func (p *Plugin) Info() sdk.Manifest {
	return sdk.Manifest{
		ID:              PluginID,
		Version:         "1.0.0",
		APIVersion:      "1.0",
		Protocol:        "modbus",
		Capabilities:    capabilityNames(),
		Transports:      []string{"tcp-ip"},
		RecoveryActions: []string{"reconnect"},
		TrustLevel:      sdk.TrustVerified,
	}
}

// Probe reports whether an endpoint is a Modbus TCP service. It is lightweight:
// it matches the endpoint type only; the connection happens in Open.
func (p *Plugin) Probe(_ context.Context, endpoint domain.Endpoint) (sdk.ProbeResult, error) {
	if endpoint.Type != domain.EndpointModbus {
		return sdk.ProbeResult{Match: false}, nil
	}
	return sdk.ProbeResult{
		Match:         true,
		Confidence:    1.0,
		IdentityHints: map[string]string{"serial": endpoint.Locator, "model": "modbus-device"},
		ChannelType:   ChannelType,
		Cost:          20,
	}, nil
}

// Capabilities returns the Modbus capability set.
func (p *Plugin) Capabilities(*domain.Channel) []domain.Capability {
	return capabilityDescriptors()
}

// Open dials the Modbus endpoint and binds the channel.
func (p *Plugin) Open(_ context.Context, channel *domain.Channel, _ domain.SessionID) error {
	c, err := dial(channel.Locator)
	if err != nil {
		return sdk.ConnectionError(domain.CodeDeviceOffline, err.Error())
	}
	p.mu.Lock()
	p.channels[channel.ID] = c
	p.mu.Unlock()
	channel.State = domain.ChannelReady
	channel.Healthy = true
	return nil
}

// Close releases the channel binding.
func (p *Plugin) Close(_ context.Context, channel *domain.Channel) error {
	p.mu.Lock()
	c := p.channels[channel.ID]
	delete(p.channels, channel.ID)
	p.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
	channel.State = domain.ChannelClosed
	channel.Healthy = false
	return nil
}

// Health performs a liveness check by reading one input register.
func (p *Plugin) Health(_ context.Context, channel *domain.Channel) error {
	c := p.client(channel.ID)
	if c == nil {
		return sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}
	if _, err := c.ReadInputRegisters(0, 1); err != nil {
		channel.Healthy = false
		return sdk.DeviceStateError(domain.CodeDeviceOffline, err.Error())
	}
	channel.Healthy = true
	return nil
}

// Recover re-dials the Modbus endpoint.
func (p *Plugin) Recover(_ context.Context, channel *domain.Channel, _ string) error {
	channel.State = domain.ChannelReconnecting
	c, err := dial(channel.Locator)
	if err != nil {
		return sdk.ConnectionError(domain.CodeDeviceOffline, err.Error())
	}
	p.mu.Lock()
	p.channels[channel.ID] = c
	p.mu.Unlock()
	channel.State = domain.ChannelReady
	channel.Healthy = true
	return nil
}

// Cancel accepts a cooperative-cancellation request.
func (p *Plugin) Cancel(_ context.Context, _ *domain.Channel, _ domain.OperationID) error {
	return nil
}

func (p *Plugin) client(id domain.ChannelID) *client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.channels[id]
}
