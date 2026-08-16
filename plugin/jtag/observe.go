package jtag

import (
	"context"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// Observe returns a read-only snapshot of the debug state.
func (p *Plugin) Observe(_ context.Context, channel *domain.Channel, req sdk.InvokeRequest) (*sdk.Observation, error) {
	dev := p.device(channel.ID)
	if dev == nil {
		return nil, sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}
	obs := &sdk.Observation{Online: dev.IsOnline(), Facts: map[string]string{}}
	if !obs.Online {
		return obs, nil
	}
	if dev.IsHalted() {
		obs.State = "halted"
	} else {
		obs.State = "running"
	}
	obs.Facts["debug.state"] = obs.State
	return obs, nil
}
