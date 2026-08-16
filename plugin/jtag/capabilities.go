package jtag

import (
	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// jtagCapabilities is the debug control-plane capability set. All are HIGH
// risk (they control the core) and require the exclusive debug resource.
var jtagCapabilities = []domain.Capability{
	{
		Name:        domain.CapabilityDebugHalt,
		Version:     "1.0",
		Description: "暂停 CPU 核心（调试控制，高危）",
		InputSchema: domain.ObjectSchema(nil, nil),
		OutputSchema: domain.ObjectSchema(nil, map[string]domain.JSONSchema{
			"halted": domain.StringSchema("是否已暂停"),
		}),
		RiskLevel:  domain.RiskHigh,
		Idempotent: true,
		ResourceRequirements: []string{
			domain.ResourceTypeDevice,
			domain.ResourceTypeDebug,
		},
	},
	{
		Name:        domain.CapabilityDebugResume,
		Version:     "1.0",
		Description: "恢复 CPU 核心运行",
		InputSchema: domain.ObjectSchema(nil, nil),
		OutputSchema: domain.ObjectSchema(nil, map[string]domain.JSONSchema{
			"halted": domain.StringSchema("是否已暂停"),
		}),
		RiskLevel:  domain.RiskHigh,
		Idempotent: true,
		ResourceRequirements: []string{
			domain.ResourceTypeDevice,
			domain.ResourceTypeDebug,
		},
	},
	{
		Name:        domain.CapabilityDebugReadMemory,
		Version:     "1.0",
		Description: "读取目标内存（字为单位）",
		InputSchema: domain.ObjectSchema(
			[]string{"address", "count"},
			map[string]domain.JSONSchema{
				"address": domain.StringSchema("起始地址（十六进制，如 0x20000000）"),
				"count":   domain.StringSchema("读取的字数"),
			},
		),
		OutputSchema: domain.StringSchema("内存值（十六进制）"),
		RiskLevel:    domain.RiskHigh,
		Idempotent:   true,
		ResourceRequirements: []string{
			domain.ResourceTypeDevice,
			domain.ResourceTypeDebug,
		},
	},
	{
		Name:        domain.CapabilityDebugWriteMemory,
		Version:     "1.0",
		Description: "写入目标内存（字为单位，高危）",
		InputSchema: domain.ObjectSchema(
			[]string{"address", "values"},
			map[string]domain.JSONSchema{
				"address": domain.StringSchema("起始地址（十六进制）"),
				"values":  domain.StringSchema("要写入的字（逗号分隔十六进制）"),
			},
		),
		OutputSchema: domain.StringSchema("写入结果"),
		RiskLevel:    domain.RiskCritical,
		Idempotent:   false,
		ResourceRequirements: []string{
			domain.ResourceTypeDevice,
			domain.ResourceTypeDebug,
		},
	},
}

func capabilityNames() []domain.CapabilityName {
	names := make([]domain.CapabilityName, 0, len(jtagCapabilities))
	for _, c := range jtagCapabilities {
		names = append(names, c.Name)
	}
	return names
}

func capabilityDescriptors() []domain.Capability {
	out := make([]domain.Capability, len(jtagCapabilities))
	copy(out, jtagCapabilities)
	return out
}
