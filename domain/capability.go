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

	// Debug (JTAG/SWD) control-plane capabilities.
	CapabilityDebugHalt        CapabilityName = "debug.halt"
	CapabilityDebugResume      CapabilityName = "debug.resume"
	CapabilityDebugReadMemory  CapabilityName = "debug.read_memory"
	CapabilityDebugWriteMemory CapabilityName = "debug.write_memory"

	// Modbus (industrial register protocol) capabilities.
	CapabilityModbusReadHolding CapabilityName = "modbus.read_holding_registers"
	CapabilityModbusReadInput   CapabilityName = "modbus.read_input_registers"
	CapabilityModbusWriteReg    CapabilityName = "modbus.write_register"
)

// Capability describes what a device can do (the semantics). A Channel is the
// implementation path; the Resolver matches the two.
//
// Description + InputSchema/OutputSchema make a capability self-describing to
// LLM/agent tool selection: {name, description, input_schema} is exactly a
// function-calling tool definition.
//
// ResourceRequirements are resource *types* (e.g. "flash", "device",
// "console"); the Resource Registry maps a type to the concrete Resource
// instance of the target device at execution time.
type Capability struct {
	Name                  CapabilityName
	Version               string
	Description           string
	InputSchema           JSONSchema
	OutputSchema          JSONSchema
	RiskLevel             RiskLevel
	Idempotent            bool
	ResourceRequirements  []string
	SupportedChannelTypes []string
}

// ToolDefinition returns the LLM-facing tool view of this capability:
// {name, description, input_schema}. It is the bridge to function-calling.
func (c *Capability) ToolDefinition() map[string]any {
	return map[string]any{
		"name":         string(c.Name),
		"description":  c.Description,
		"input_schema": c.InputSchema,
	}
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
