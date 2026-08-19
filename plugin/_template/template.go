// Package template is a from-scratch skeleton for a new Channel Plugin.
//
// 接入步骤（见 docs/PLUGIN-SPEC.md §11）：
//   1. 复制整个目录 plugin/_template/ → plugin/<your-proto>/
//   2. 全局替换占位符 `template` → 你的协议名（包名、PluginID、ChannelType）
//   3. 替换 Probe 里的 EndpointType 为你自己的类型（或新增 domain 常量）
//   4. 在 capabilities.go 声明能力，在 invoke.go 写能力→协议动作映射
//   5. 在 runtime/bootstrap.go 注册一行
//   6. 写 AssertPluginContract 合同测试
//
// 本模板是一个可工作的最小插件（device.info.get + device.execute），
// 由 fake.Farm 支撑，供接入方照抄后替换成真实协议实现。
package template

import (
	"context"
	"sync"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// PluginID is the stable, globally-unique plugin identity. 换成你的协议名，
// 约定 protocol.<name>，永不改名。
const PluginID = "protocol.template"

// ChannelType is the runtime channel type this plugin creates.
const ChannelType = "template"

// Plugin implements sdk.Plugin against an in-memory fake device farm.
// 真实实现时，把 *fake.Farm 换成你的底层客户端/连接池/设备表。
type Plugin struct {
	farm *fake.Farm

	mu       sync.RWMutex
	channels map[domain.ChannelID]*fake.Device
}

// New builds the plugin backed by farm.
func New(farm *fake.Farm) *Plugin {
	return &Plugin{
		farm:     farm,
		channels: map[domain.ChannelID]*fake.Device{},
	}
}

// Info returns the plugin Manifest.
func (p *Plugin) Info() sdk.Manifest {
	return sdk.Manifest{
		ID:              PluginID,
		Version:         "1.0.0",
		APIVersion:      "1.0",
		Protocol:        "template", // 换成你的协议名
		Capabilities:    capabilityNames(),
		Transports:      []string{"tcp-ip"}, // 换成你的传输形态
		RecoveryActions: []string{"reconnect"},
		TrustLevel:      sdk.TrustVerified,
	}
}

// Probe reports whether an endpoint belongs to this plugin. 必须轻量、无副作用。
// 把 EndpointUSBADB 换成你自己的 EndpointType（见 domain/endpoint.go，或新增常量）。
func (p *Plugin) Probe(_ context.Context, endpoint domain.Endpoint) (sdk.ProbeResult, error) {
	if endpoint.Type != domain.EndpointUSBADB {
		return sdk.ProbeResult{Match: false}, nil
	}
	dev := p.farm.ByLocator(endpoint.Locator)
	if dev == nil {
		return sdk.ProbeResult{Match: false}, nil
	}
	return sdk.ProbeResult{
		Match:         true,
		Confidence:    1.0,
		IdentityHints: dev.Identity(),
		ChannelType:   ChannelType,
		Cost:          10,
	}, nil
}

// Capabilities returns the capability set for a channel.
func (p *Plugin) Capabilities(*domain.Channel) []domain.Capability {
	return capabilityDescriptors()
}

// Open establishes the channel and marks it READY+Healthy.
func (p *Plugin) Open(_ context.Context, channel *domain.Channel, _ domain.SessionID) error {
	return p.bind(channel)
}

// Close tears down the channel.
func (p *Plugin) Close(_ context.Context, channel *domain.Channel) error {
	p.mu.Lock()
	delete(p.channels, channel.ID)
	p.mu.Unlock()
	channel.State = domain.ChannelClosed
	channel.Healthy = false
	return nil
}

// Health performs a fast, low-side-effect liveness check.
func (p *Plugin) Health(_ context.Context, channel *domain.Channel) error {
	dev := p.device(channel.ID)
	if dev == nil {
		return sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}
	if !dev.IsOnline() {
		channel.Healthy = false
		return sdk.DeviceStateError(domain.CodeDeviceOffline, "device offline")
	}
	channel.Healthy = true
	return nil
}

// Cancel accepts a cooperative-cancellation request (optional sdk.Canceller).
func (p *Plugin) Cancel(_ context.Context, _ *domain.Channel, _ domain.OperationID) error {
	return nil
}

// Recover re-binds a lost channel. reason 由 Core 的 Recovery Policy 传入。
func (p *Plugin) Recover(_ context.Context, channel *domain.Channel, _ string) error {
	channel.State = domain.ChannelReconnecting
	return p.bind(channel)
}

// bind resolves a channel to its device via the endpoint locator.
func (p *Plugin) bind(channel *domain.Channel) error {
	dev := p.farm.ByLocator(channel.Locator)
	if dev == nil {
		return sdk.ConnectionError(domain.CodeDeviceOffline, "no device at locator "+channel.Locator)
	}
	if !dev.IsOnline() {
		return sdk.DeviceStateError(domain.CodeDeviceOffline, "device is offline")
	}
	p.mu.Lock()
	p.channels[channel.ID] = dev
	p.mu.Unlock()
	channel.State = domain.ChannelReady
	channel.Healthy = true
	return nil
}

func (p *Plugin) device(id domain.ChannelID) *fake.Device {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.channels[id]
}
