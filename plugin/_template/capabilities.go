package template

import (
	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// capabilities is the plugin's capability set. Description + input/output
// schema make each capability self-describing to LLM tool selection.
// 换成你自己的能力：优先复用 domain.Capability* 常量，协议独有的才新增。
var capabilities = []domain.Capability{
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
	names := make([]domain.CapabilityName, 0, len(capabilities))
	for _, c := range capabilities {
		names = append(names, c.Name)
	}
	return names
}

func capabilityDescriptors() []domain.Capability {
	out := make([]domain.Capability, len(capabilities))
	copy(out, capabilities)
	return out
}
