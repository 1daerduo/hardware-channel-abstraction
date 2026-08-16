package modbus

import (
	"context"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// Observe returns a read-only snapshot of the Modbus device.
func (p *Plugin) Observe(_ context.Context, channel *domain.Channel, req sdk.InvokeRequest) (*sdk.Observation, error) {
	c := p.client(channel.ID)
	if c == nil {
		return nil, sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}
	obs := &sdk.Observation{Facts: map[string]string{}}
	if _, err := c.ReadInputRegisters(0, 1); err != nil {
		obs.Online = false
		return obs, nil
	}
	obs.Online = true
	obs.State = "online"
	obs.Facts["modbus.state"] = "online"
	return obs, nil
}
