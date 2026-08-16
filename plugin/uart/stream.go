package uart

import (
	"context"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// Stream implements sdk.Streamer: it opens a live console/log stream when the
// underlying transport supports streaming. Transports that do not stream (the
// fake simulator) return a normalized error instead.
func (p *Plugin) Stream(ctx context.Context, channel *domain.Channel, req sdk.StreamRequest) (sdk.Stream, error) {
	dev := p.device(channel.ID)
	if dev == nil {
		return nil, sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}
	provider, ok := dev.(sdk.StreamProvider)
	if !ok {
		return nil, sdk.Error(domain.CodeUnsupportedCap, domain.CategoryValidation,
			"transport does not support streaming for "+string(req.Capability))
	}
	return provider.OpenStream(ctx)
}
