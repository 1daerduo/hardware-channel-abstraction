package uart

import (
	"context"

	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/plugin/sdk"
)

// Invoke maps a unified operation to serial-console actions. The device is
// checked for reachability before any command is sent.
func (p *Plugin) Invoke(_ context.Context, channel *domain.Channel, req sdk.InvokeRequest) (*sdk.InvokeResult, error) {
	dev := p.device(channel.ID)
	if dev == nil {
		return nil, sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}
	if !dev.IsOnline() {
		return nil, sdk.DeviceStateError(domain.CodeDeviceOffline, "device offline")
	}

	switch req.Capability {
	case domain.CapabilityConsole:
		out, err := dev.Console()
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   out,
			Evidence: []domain.Evidence{*evidenceFor(req, "console.capture", "ok")},
		}, nil

	case domain.CapabilityLog:
		out, err := dev.Log()
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   out,
			Evidence: []domain.Evidence{*evidenceFor(req, "log.source", "uart")},
		}, nil

	case domain.CapabilityReset:
		if err := dev.Reset(); err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   "reset",
			Evidence: []domain.Evidence{*evidenceFor(req, "reset.state", dev.CurrentBootState())},
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

	default:
		return nil, sdk.Error(domain.CodeUnsupportedCap, domain.CategoryValidation,
			"unsupported capability "+string(req.Capability))
	}
}

// mapErr wraps a transport/protocol error. Reachability is checked before
// Invoke, so any error here is a protocol/execution failure.
func mapErr(err error) *domain.Error {
	return sdk.ProtocolError(domain.CodeInternal, err.Error(), err.Error())
}

func evidenceFor(req sdk.InvokeRequest, name, value string) *domain.Evidence {
	ev := domain.NewEvidence(name, value)
	ev.OperationID = req.OperationID
	return ev
}
