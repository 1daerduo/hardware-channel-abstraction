package adb

import (
	"example.com/embedded-loop-channel/domain"
)

// adbCapabilities is the plugin's static capability set (declared in the
// Manifest). It is read-only; callers must not mutate it.
var adbCapabilities = []domain.Capability{
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
	{
		Name:       domain.CapabilityFlash,
		Version:    "1.0",
		RiskLevel:  domain.RiskHigh,
		Idempotent: false,
		ResourceRequirements: []string{
			domain.ResourceTypeDevice,
			domain.ResourceTypeFlash,
		},
	},
	{
		Name:       domain.CapabilityFileRead,
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
		Name:                 domain.CapabilityExecute,
		Version:              "1.0",
		RiskLevel:            domain.RiskHigh,
		Idempotent:           false,
		ResourceRequirements: []string{domain.ResourceTypeDevice},
	},
}

// capabilityNames returns the capability names advertised in the manifest.
func capabilityNames() []domain.CapabilityName {
	names := make([]domain.CapabilityName, 0, len(adbCapabilities))
	for _, c := range adbCapabilities {
		names = append(names, c.Name)
	}
	return names
}

// capabilityDescriptors returns the plugin's capability set (Design doc 06).
func capabilityDescriptors() []domain.Capability {
	out := make([]domain.Capability, len(adbCapabilities))
	copy(out, adbCapabilities)
	return out
}

// capabilityByName returns a copy of the descriptor or nil.
func capabilityByName(name domain.CapabilityName) *domain.Capability {
	for _, c := range adbCapabilities {
		if c.Name == name {
			c := c
			return &c
		}
	}
	return nil
}
