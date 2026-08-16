package tcp

import (
	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// tcpCapabilities maps a TCP console's commands to unified capabilities.
// device.info.get / device.reboot / device.execute overlap ADB intentionally,
// proving one capability can be served by a network transport too.
var tcpCapabilities = []domain.Capability{
	{
		Name:        domain.CapabilityInfoGet,
		Version:     "1.0",
		Description: "读取设备信息（型号、固件版本、运行状态）",
		InputSchema: domain.ObjectSchema(nil, nil),
		OutputSchema: domain.ObjectSchema(nil, map[string]domain.JSONSchema{
			"model":    domain.StringSchema("设备型号"),
			"firmware": domain.StringSchema("固件版本"),
			"state":    domain.StringSchema("运行状态"),
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
			"state": domain.StringSchema("重启后的状态"),
		}),
		RiskLevel:            domain.RiskMedium,
		Idempotent:           false,
		ResourceRequirements: []string{domain.ResourceTypeDevice},
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
	{
		Name:         domain.CapabilityConsole,
		Version:      "1.0",
		Description:  "读取设备控制台输出",
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
}

func capabilityNames() []domain.CapabilityName {
	names := make([]domain.CapabilityName, 0, len(tcpCapabilities))
	for _, c := range tcpCapabilities {
		names = append(names, c.Name)
	}
	return names
}

func capabilityDescriptors() []domain.Capability {
	out := make([]domain.Capability, len(tcpCapabilities))
	copy(out, tcpCapabilities)
	return out
}
