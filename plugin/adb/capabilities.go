package adb

import (
	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// adbCapabilities is the plugin's static capability set (declared in the
// Manifest). It is read-only; callers must not mutate it. Description +
// input/output schema make each capability self-describing to LLM tool
// selection.
var adbCapabilities = []domain.Capability{
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
	{
		Name:        domain.CapabilityFlash,
		Version:     "1.0",
		Description: "刷写固件镜像到设备的指定分区",
		InputSchema: domain.ObjectSchema(
			[]string{"partition", "image", "version"},
			map[string]domain.JSONSchema{
				"partition": domain.StringSchema("目标分区，如 boot / system"),
				"image":     domain.StringSchema("镜像文件名"),
				"version":   domain.StringSchema("镜像版本号，用于后置校验"),
			},
		),
		OutputSchema: domain.ObjectSchema(nil, map[string]domain.JSONSchema{
			"partition": domain.StringSchema("已刷写的分区"),
			"version":   domain.StringSchema("校验后的分区版本"),
		}),
		RiskLevel:  domain.RiskHigh,
		Idempotent: false,
		ResourceRequirements: []string{
			domain.ResourceTypeDevice,
			domain.ResourceTypeFlash,
		},
	},
	{
		Name:        domain.CapabilityFileRead,
		Version:     "1.0",
		Description: "读取设备上的文件内容",
		InputSchema: domain.ObjectSchema(
			[]string{"path"},
			map[string]domain.JSONSchema{"path": domain.StringSchema("设备上的文件路径")},
		),
		OutputSchema: domain.StringSchema("文件内容"),
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
