package uart

import (
	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// uartCapabilities is the plugin's static capability set. device.log is
// intentionally shared with the ADB plugin to exercise multi-channel
// resolution; device.console and device.reset are UART-specific.
var uartCapabilities = []domain.Capability{
	{
		Name:         domain.CapabilityConsole,
		Version:      "1.0",
		Description:  "读取设备串口控制台输出",
		InputSchema:  domain.ObjectSchema(nil, nil),
		OutputSchema: domain.StringSchema("控制台输出"),
		RiskLevel:    domain.RiskLow,
		Idempotent:   true,
	},
	{
		Name:         domain.CapabilityLog,
		Version:      "1.0",
		Description:  "读取设备日志",
		InputSchema:  domain.ObjectSchema(nil, nil),
		OutputSchema: domain.StringSchema("日志内容"),
		RiskLevel:    domain.RiskLow,
		Idempotent:   true,
	},
	{
		Name:         domain.CapabilityReset,
		Version:      "1.0",
		Description:  "硬件复位设备（高危）",
		InputSchema:  domain.ObjectSchema(nil, nil),
		OutputSchema: domain.StringSchema("复位后的启动状态"),
		RiskLevel:    domain.RiskHigh,
		Idempotent:   false,
		ResourceRequirements: []string{
			domain.ResourceTypeDevice,
			domain.ResourceTypeDebug,
		},
	},
	{
		Name:        domain.CapabilityExecute,
		Version:     "1.0",
		Description: "在设备上执行命令（高危，需授权与审计）",
		InputSchema: domain.ObjectSchema(
			[]string{"command"},
			map[string]domain.JSONSchema{"command": domain.StringSchema("要执行的命令")},
		),
		OutputSchema:         domain.StringSchema("命令输出"),
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
