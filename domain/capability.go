package domain

// RiskLevel drives Policy, Permission and Approval. Risk is not Permission:
// a principal may hold a capability permission while CRITICAL actions still
// require approval.
type RiskLevel string

const (
	RiskLow      RiskLevel = "LOW"
	RiskMedium   RiskLevel = "MEDIUM"
	RiskHigh     RiskLevel = "HIGH"
	RiskCritical RiskLevel = "CRITICAL"
)

// Standard capability names. Upper layers reference capabilities, never
// protocol commands.
const (
	CapabilityInfoGet  CapabilityName = "device.info.get"
	CapabilityReboot   CapabilityName = "device.reboot"
	CapabilityFlash    CapabilityName = "device.flash"
	CapabilityExecute  CapabilityName = "device.execute"
	CapabilityFileRead CapabilityName = "device.file.read"
	CapabilityLog      CapabilityName = "device.log"
	CapabilityConsole  CapabilityName = "device.console"
	CapabilityReset    CapabilityName = "device.reset"
)

// Capability describes what a device can do (the semantics). A Channel is the
// implementation path; the Resolver matches the two.
//
// ResourceRequirements are resource *types* (e.g. "flash", "device",
// "console"); the Resource Registry maps a type to the concrete Resource
// instance of the target device at execution time.
type Capability struct {
	Name                  CapabilityName
	Version               string
	InputSchema           string
	OutputSchema          string
	RiskLevel             RiskLevel
	Idempotent            bool
	ResourceRequirements  []string
	SupportedChannelTypes []string
}

// RequireResource records a resource type this capability needs (e.g. the
// flash subsystem, or exclusive device access).
func (c *Capability) RequireResource(typ string) {
	c.ResourceRequirements = append(c.ResourceRequirements, typ)
}

// SupportedByChannel reports whether this capability can run over the channel
// type. An empty SupportedChannelTypes means "any channel".
func (c *Capability) SupportedByChannel(channelType string) bool {
	if len(c.SupportedChannelTypes) == 0 {
		return true
	}
	for _, t := range c.SupportedChannelTypes {
		if t == channelType {
			return true
		}
	}
	return false
}
