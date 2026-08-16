package modbus

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// Invoke maps a unified operation to Modbus register transactions.
func (p *Plugin) Invoke(_ context.Context, channel *domain.Channel, req sdk.InvokeRequest) (*sdk.InvokeResult, error) {
	c := p.client(channel.ID)
	if c == nil {
		return nil, sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}

	addr, err := parseU16(req.Parameters["address"])
	if err != nil {
		return nil, sdk.Error(domain.CodeInvalidInput, domain.CategoryValidation, err.Error())
	}

	switch req.Capability {
	case domain.CapabilityModbusReadHolding:
		qty := parseQty(req.Parameters["quantity"])
		vals, err := c.ReadHoldingRegisters(addr, qty)
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   formatRegs(vals),
			Evidence: []domain.Evidence{*evidenceFor(req, "modbus.kind", "holding")},
		}, nil

	case domain.CapabilityModbusReadInput:
		qty := parseQty(req.Parameters["quantity"])
		vals, err := c.ReadInputRegisters(addr, qty)
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   formatRegs(vals),
			Evidence: []domain.Evidence{*evidenceFor(req, "modbus.kind", "input")},
		}, nil

	case domain.CapabilityModbusWriteReg:
		val, err := parseU16(req.Parameters["value"])
		if err != nil {
			return nil, sdk.Error(domain.CodeInvalidInput, domain.CategoryValidation, err.Error())
		}
		if err := c.WriteRegister(addr, val); err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   fmt.Sprintf("wrote 0x%04x @ %d", val, addr),
			Evidence: []domain.Evidence{*evidenceFor(req, "modbus.address", fmt.Sprintf("%d", addr))},
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

func parseU16(s string) (uint16, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return 0, fmt.Errorf("无效寄存器值 %q", s)
	}
	return uint16(v), nil
}

func parseQty(s string) uint16 {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
		return uint16(n)
	}
	return 1
}

func formatRegs(vals []uint16) string {
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, fmt.Sprintf("0x%04x", v))
	}
	return strings.Join(parts, "\n")
}
