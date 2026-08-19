package template

import (
	"context"
	"errors"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// Invoke maps a unified operation to protocol actions and returns a
// normalized result with Evidence. Raw errors must be translated to
// domain.Error so protocol text never leaks into Core.
func (p *Plugin) Invoke(_ context.Context, channel *domain.Channel, req sdk.InvokeRequest) (*sdk.InvokeResult, error) {
	dev := p.device(channel.ID)
	if dev == nil {
		return nil, sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}

	switch req.Capability {
	case domain.CapabilityInfoGet:
		info, err := dev.Info()
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output: formatKV(info),
			Evidence: []domain.Evidence{
				*evidenceFor(req, "info.model", info["model"]),
			},
		}, nil

	case domain.CapabilityExecute:
		out, err := dev.Execute(req.Parameters["command"])
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{Output: out}, nil

	default:
		return nil, sdk.Error(domain.CodeUnsupportedCap, domain.CategoryValidation,
			"unsupported capability "+string(req.Capability))
	}
}

// mapErr translates device errors into the unified taxonomy. 换成你自己的
// 底层错误类型映射（errors.Is 判断 + 对应 category）。
func mapErr(err error) *domain.Error {
	if errors.Is(err, fake.ErrOffline) {
		return sdk.DeviceStateError(domain.CodeDeviceOffline, "device offline")
	}
	return sdk.ProtocolError(domain.CodeInternal, err.Error(), err.Error())
}

func evidenceFor(req sdk.InvokeRequest, name, value string) *domain.Evidence {
	ev := domain.NewEvidence(name, value)
	ev.OperationID = req.OperationID
	return ev
}
