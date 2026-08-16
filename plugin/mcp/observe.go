package mcp

import (
	"context"

	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/plugin/sdk"
)

// Observe returns a read-only snapshot of the device's postcondition state.
func (p *Plugin) Observe(_ context.Context, channel *domain.Channel, req sdk.InvokeRequest) (*sdk.Observation, error) {
	dev := p.device(channel.ID)
	if dev == nil {
		return nil, sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}
	obs := &sdk.Observation{Online: dev.IsOnline(), Facts: map[string]string{}}
	if !obs.Online {
		return obs, nil
	}
	obs.State = dev.CurrentBootState()
	obs.Facts["device.state"] = dev.CurrentBootState()
	return obs, nil
}
