package mcp

import (
	"example.com/embedded-loop-channel/domain"
)

// mcpCapabilities maps MCP tools to unified capabilities. device.info.get and
// device.reboot overlap ADB intentionally, proving one capability can be
// served by multiple protocol channels.
var mcpCapabilities = []domain.Capability{
	{
		Name:       domain.CapabilityInfoGet,
		Version:    "1.0",
		RiskLevel:  domain.RiskLow,
		Idempotent: true,
	},
	{
		Name:                 domain.CapabilityReboot,
		Version:              "1.0",
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
