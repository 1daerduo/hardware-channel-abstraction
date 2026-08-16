package modbus

import (
	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// modbusCapabilities maps Modbus function codes to unified capabilities.
var modbusCapabilities = []domain.Capability{
	{
		Name:        domain.CapabilityModbusReadHolding,
		Version:     "1.0",
		Description: "读取保持寄存器（Modbus FC 0x03）",
		InputSchema: domain.ObjectSchema(
			[]string{"address", "quantity"},
			map[string]domain.JSONSchema{
				"address":  domain.StringSchema("起始寄存器地址（十进制）"),
				"quantity": domain.StringSchema("读取数量"),
			},
		),
		OutputSchema: domain.StringSchema("寄存器值（十六进制）"),
		RiskLevel:    domain.RiskLow,
		Idempotent:   true,
	},
	{
		Name:        domain.CapabilityModbusReadInput,
		Version:     "1.0",
		Description: "读取输入寄存器（Modbus FC 0x04）",
		InputSchema: domain.ObjectSchema(
			[]string{"address", "quantity"},
			map[string]domain.JSONSchema{
				"address":  domain.StringSchema("起始寄存器地址"),
				"quantity": domain.StringSchema("读取数量"),
			},
		),
		OutputSchema: domain.StringSchema("寄存器值（十六进制）"),
		RiskLevel:    domain.RiskLow,
		Idempotent:   true,
	},
	{
		Name:        domain.CapabilityModbusWriteReg,
		Version:     "1.0",
		Description: "写单个保持寄存器（Modbus FC 0x06）",
		InputSchema: domain.ObjectSchema(
			[]string{"address", "value"},
			map[string]domain.JSONSchema{
				"address": domain.StringSchema("寄存器地址"),
				"value":   domain.StringSchema("写入值（十六进制）"),
			},
		),
		OutputSchema: domain.StringSchema("写入结果"),
		RiskLevel:    domain.RiskMedium,
		Idempotent:   false,
	},
}

func capabilityNames() []domain.CapabilityName {
	names := make([]domain.CapabilityName, 0, len(modbusCapabilities))
	for _, c := range modbusCapabilities {
		names = append(names, c.Name)
	}
	return names
}

func capabilityDescriptors() []domain.Capability {
	out := make([]domain.Capability, len(modbusCapabilities))
	copy(out, modbusCapabilities)
	return out
}
