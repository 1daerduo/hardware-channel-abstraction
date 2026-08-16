package jtag

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// Invoke maps a unified debug operation to fake-JTAG actions.
func (p *Plugin) Invoke(_ context.Context, channel *domain.Channel, req sdk.InvokeRequest) (*sdk.InvokeResult, error) {
	dev := p.device(channel.ID)
	if dev == nil {
		return nil, sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}
	if !dev.IsOnline() {
		return nil, sdk.DeviceStateError(domain.CodeDeviceOffline, "device offline")
	}

	switch req.Capability {
	case domain.CapabilityDebugHalt:
		if err := dev.Halt(); err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   "halted",
			Evidence: []domain.Evidence{*evidenceFor(req, "debug.halted", "true")},
		}, nil

	case domain.CapabilityDebugResume:
		if err := dev.Resume(); err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   "running",
			Evidence: []domain.Evidence{*evidenceFor(req, "debug.halted", "false")},
		}, nil

	case domain.CapabilityDebugReadMemory:
		addr, err := parseAddr(req.Parameters["address"])
		if err != nil {
			return nil, sdk.Error(domain.CodeInvalidInput, domain.CategoryValidation, err.Error())
		}
		count := 1
		if c, err := strconv.Atoi(req.Parameters["count"]); err == nil && c > 0 {
			count = c
		}
		words, err := dev.ReadMemory(addr, count)
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   formatWords(words),
			Evidence: []domain.Evidence{*evidenceFor(req, "debug.address", fmt.Sprintf("0x%x", addr))},
		}, nil

	case domain.CapabilityDebugWriteMemory:
		addr, err := parseAddr(req.Parameters["address"])
		if err != nil {
			return nil, sdk.Error(domain.CodeInvalidInput, domain.CategoryValidation, err.Error())
		}
		values, err := parseWords(req.Parameters["values"])
		if err != nil {
			return nil, sdk.Error(domain.CodeInvalidInput, domain.CategoryValidation, err.Error())
		}
		if err := dev.WriteMemory(addr, values); err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   fmt.Sprintf("wrote %d word(s) @0x%x", len(values), addr),
			Evidence: []domain.Evidence{*evidenceFor(req, "debug.address", fmt.Sprintf("0x%x", addr))},
		}, nil

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

func parseAddr(s string) (uint32, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("无效地址 %q", s)
	}
	return uint32(v), nil
}

func parseWords(s string) ([]uint32, error) {
	var out []uint32
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimPrefix(tok, "0x"), 16, 32)
		if err != nil {
			return nil, fmt.Errorf("无效字 %q", tok)
		}
		out = append(out, uint32(v))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("values 为空")
	}
	return out, nil
}

func formatWords(words []uint32) string {
	parts := make([]string, 0, len(words))
	for _, w := range words {
		parts = append(parts, fmt.Sprintf("0x%08x", w))
	}
	return strings.Join(parts, "\n")
}
