package mcp

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// Invoke maps a unified operation to an MCP tool call. The tool name is
// recorded as evidence, demonstrating protocol-level observability without
// leaking the MCP wire format into Core.
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
				*evidenceFor(req, "mcp.tool", "get_device_info"),
				*evidenceFor(req, "info.model", info["model"]),
			},
		}, nil

	case domain.CapabilityReboot:
		if err := dev.Reboot(); err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   "rebooted",
			Evidence: []domain.Evidence{*evidenceFor(req, "mcp.tool", "reboot_device")},
		}, nil

	default:
		return nil, sdk.Error(domain.CodeUnsupportedCap, domain.CategoryValidation,
			"unsupported capability "+string(req.Capability))
	}
}

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

func formatKV(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(m[k])
		b.WriteString("\n")
	}
	return b.String()
}
