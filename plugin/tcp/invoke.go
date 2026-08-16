package tcp

import (
	"context"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// Invoke maps a unified operation to console commands.
func (p *Plugin) Invoke(_ context.Context, channel *domain.Channel, req sdk.InvokeRequest) (*sdk.InvokeResult, error) {
	dev := p.device(channel.ID)
	if dev == nil {
		return nil, sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}
	if !dev.IsOnline() {
		return nil, sdk.DeviceStateError(domain.CodeDeviceOffline, "device offline")
	}

	switch req.Capability {
	case domain.CapabilityInfoGet:
		out, err := dev.Execute("get_device_info")
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   out,
			Evidence: []domain.Evidence{*evidenceFor(req, "info.source", "tcp")},
		}, nil

	case domain.CapabilityReboot:
		out, err := dev.Execute("reboot")
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   out,
			Evidence: []domain.Evidence{*evidenceFor(req, "reboot.state", "rebooted")},
		}, nil

	case domain.CapabilityExecute:
		out, err := dev.Execute(req.Parameters["command"])
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   out,
			Evidence: []domain.Evidence{*evidenceFor(req, "console.command", req.Parameters["command"])},
		}, nil

	case domain.CapabilityConsole:
		out, err := dev.Console()
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{Output: out}, nil

	case domain.CapabilityLog:
		out, err := dev.Log()
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{Output: out}, nil

	default:
		return nil, sdk.Error(domain.CodeUnsupportedCap, domain.CategoryValidation,
			"unsupported capability "+string(req.Capability))
	}
}

func mapErr(err error) *domain.Error {
	return sdk.ProtocolError(domain.CodeInternal, err.Error(), err.Error())
}

func evidenceFor(req sdk.InvokeRequest, name, value string) *domain.Evidence {
	ev := domain.NewEvidence(name, value)
	ev.OperationID = req.OperationID
	return ev
}
