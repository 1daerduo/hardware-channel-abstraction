package mcp

import (
	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// mcpCapabilities maps MCP tools to unified capabilities. device.info.get and
// device.reboot overlap ADB intentionally, proving one capability can be
// served by multiple protocol channels.
var mcpCapabilities = []domain.Capability{
	{
		Name:        domain.CapabilityInfoGet,
		Version:     "1.0",
		Description: "读取设备信息（序列号、型号、固件版本、启动状态）",
		InputSchema: domain.ObjectSchema(nil, nil),
		OutputSchema: domain.ObjectSchema(nil, map[string]domain.JSONSchema{
			"serial":   domain.StringSchema("设备序列号"),
			"model":    domain.StringSchema("设备型号"),
			"firmware": domain.StringSchema("固件版本"),
			"state":    domain.StringSchema("启动状态"),
		}),
		RiskLevel:  domain.RiskLow,
		Idempotent: true,
	},
	{
		Name:        domain.CapabilityReboot,
		Version:     "1.0",
		Description: "重启设备",
		InputSchema: domain.ObjectSchema(nil, nil),
		OutputSchema: domain.ObjectSchema(nil, map[string]domain.JSONSchema{
			"state": domain.StringSchema("重启后的启动状态"),
		}),
		RiskLevel:            domain.RiskMedium,
		Idempotent:           false,
		ResourceRequirements: []string{domain.ResourceTypeDevice},
	},
}

func capabilityNames() []domain.CapabilityName {
	names := make([]domain.CapabilityName, 0, len(mcpCapabilities))
	for _, c := range mcpCapabilities {
		names = append(names, c.Name)
	}
	return names
}

func capabilityDescriptors() []domain.Capability {
	out := make([]domain.Capability, len(mcpCapabilities))
	copy(out, mcpCapabilities)
	return out
}
