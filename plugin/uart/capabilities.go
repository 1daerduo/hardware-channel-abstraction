package uart

import (
	"example.com/embedded-loop-channel/domain"
)

// uartCapabilities is the plugin's static capability set. device.log is
// intentionally shared with the ADB plugin to exercise multi-channel
// resolution; device.console and device.reset are UART-specific.
var uartCapabilities = []domain.Capability{
	{
		Name:       domain.CapabilityConsole,
		Version:    "1.0",
		RiskLevel:  domain.RiskLow,
		Idempotent: true,
	},
	{
		Name:       domain.CapabilityLog,
		Version:    "1.0",
		RiskLevel:  domain.RiskLow,
		Idempotent: true,
	},
	{
		Name:       domain.CapabilityReset,
		Version:    "1.0",
		RiskLevel:  domain.RiskHigh,
		Idempotent: false,
		ResourceRequirements: []string{
			domain.ResourceTypeDevice,
			domain.ResourceTypeDebug,
		},
	},
	{
		Name:                 domain.CapabilityExecute,
		Version:              "1.0",
		RiskLevel:            domain.RiskHigh,
		Idempotent:           false,
		ResourceRequirements: []string{domain.ResourceTypeDevice},
	},
}

func capabilityNames() []domain.CapabilityName {
	names := make([]domain.CapabilityName, 0, len(uartCapabilities))
	for _, c := range uartCapabilities {
		names = append(names, c.Name)
	}
	return names
}

func capabilityDescriptors() []domain.Capability {
	out := make([]domain.Capability, len(uartCapabilities))
	copy(out, uartCapabilities)
	return out
}
