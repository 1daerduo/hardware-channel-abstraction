---
name: add-channel-protocol
description: 新增一种设备接入协议或连接方式（TCP、BLE、JTAG、Vendor RPC、HTTP、SSH 等）到通道抽象层。当用户要接入新的物理/虚拟连接、新增协议适配、或问「怎么加一个新设备类型」时使用。先评估连接形态，再按 Plugin SPI 实现、注册、写合同测试，全程不碰 core/sdk/domain。
---

# 新增协议 / 连接方式

## 何时使用

用户说「接入 X 设备」「新增 X 协议」「支持 TCP/BLE/JTAG 连接」「加一个 Vendor 适配」等。

## 核心承诺（不可违背）

- 新协议 = **新增 `plugin/<proto>/` 包 + `runtime` 注册一行 + 一条合同测试**。
- **绝不修改 `core/`、`sdk/`、`domain/`**。若发现自己要改它们，说明抽象选错了，回到第 0 步。
- 架构门禁（`tests/arch`）会拦截：core/sdk 出现协议名字面量、core 依赖具体插件、插件依赖 core。

## 第 0 步：评估连接形态（决定实现路径）

| 连接形态 | 特征 | 实现路径 |
|---|---|---|
| **字节流控制台** | 发命令、读回显（串口 / TCP / SSH / 蓝牙 SPP） | 实现 `sdk.ConsoleDevice`；可复制 `transport/serial` 的泵+流式 |
| **请求/响应（结构化）** | GATT / RPC / HTTP API，非裸字节流 | 直接实现 `sdk.Plugin`，底层用对应库 |
| **远程服务** | MCP / 云端 API | 参考 `plugin/mcp`（tool→capability 映射） |

> 判断口诀：设备能不能「敲命令回车看回显」？能 → 字节流路径；只能「发结构化请求等响应」→ Plugin 直连路径。

## 第 1 步：新建插件包

```go
// plugin/<proto>/<proto>.go
package <proto>

const PluginID     = "protocol.<proto>"   // 稳定、全局唯一，永不改名
const ChannelType  = "<proto>"            // 运行时通道类型
```

## 第 2 步：实现 `sdk.Plugin`（必选方法）

```go
type Plugin struct { /* 底层 client / 连接池 / 设备表 */ }

func (p *Plugin) Info() sdk.Manifest {
    return sdk.Manifest{
        ID: PluginID, Version: "1.0.0", APIVersion: "1.0", Protocol: "<proto>",
        Capabilities: []domain.CapabilityName{/* 见第3步 */},
        Transports:   []string{"<transport>"},
        RecoveryActions: []string{"reconnect"},
        TrustLevel:   sdk.TrustVerified,
    }
}

// Probe：无副作用地判断 endpoint 是否属于本协议
func (p *Plugin) Probe(ctx, ep domain.Endpoint) (sdk.ProbeResult, error) {
    if ep.Type != domain.Endpoint<YOURTYPE> { return sdk.ProbeResult{Match: false}, nil }
    dev := p.lookup(ep.Locator)          // 按 locator 找设备
    if dev == nil { return sdk.ProbeResult{Match: false}, nil }
    return sdk.ProbeResult{Match: true, Confidence: 1.0, IdentityHints: dev.Identity(), ChannelType: ChannelType, Cost: 10}, nil
}

func (p *Plugin) Capabilities(*domain.Channel) []domain.Capability { return capabilityDescriptors() }

func (p *Plugin) Open(ctx, ch, _) error   { /* 连接并绑定，成功后 ch.State=READY, ch.Healthy=true */ }
func (p *Plugin) Close(ctx, ch) error     { /* 断开，ch.State=CLOSED */ }
func (p *Plugin) Health(ctx, ch) error    { /* 快速低副作用探活；离线返回 DeviceStateError */ }

func (p *Plugin) Invoke(ctx, ch, req sdk.InvokeRequest) (*sdk.InvokeResult, error) {
    dev := p.device(ch.ID)
    switch req.Capability {
    case domain.CapabilityInfoGet:
        out, err := dev.Info()          // 统一能力 → 协议动作
        if err != nil { return nil, mapErr(err) }
        return &sdk.InvokeResult{Output: out, Evidence: []domain.Evidence{...}}, nil
    // ... 其它 capability
    }
}

func (p *Plugin) Observe(ctx, ch, req) (*sdk.Observation, error) {
    // 只读观测：返回 Online + 后置条件 Facts（供恢复对账）
    return &sdk.Observation{Online: dev.IsOnline(), Facts: map[string]string{...}}, nil
}

func (p *Plugin) Recover(ctx, ch, reason) error {
    // reason=="reconnect" 重连；reason=="device_recovery" 高危恢复（受 Core 预算约束）
    return p.bind(ch)
}
```

可选方法（按需实现，Core 用类型断言探测）：

```go
func (p *Plugin) Cancel(ctx, ch, opID) error { ... }        // sdk.Canceller
func (p *Plugin) Stream(ctx, ch, req) (sdk.Stream, error) { ... } // sdk.Streamer
```

## 第 3 步：Capability 映射（统一语义 → 协议动作）

```go
// plugin/<proto>/capabilities.go
var caps = []domain.Capability{
    {Name: domain.CapabilityInfoGet, Version: "1.0", RiskLevel: domain.RiskLow,  Idempotent: true},
    {Name: domain.CapabilityReboot,  Version: "1.0", RiskLevel: domain.RiskMedium, Idempotent: false,
     ResourceRequirements: []string{domain.ResourceTypeDevice}},
}
```

- 尽量复用已有 `domain.Capability*` 常量（device.info.get / reboot / flash / execute / log / reset / console…）。
- 只有协议独有的能力才新加 `domain.CapabilityXxx` 常量（在 `domain/capability.go`，这是唯一可改的 domain 文件）。

## 第 4 步：错误归一化 + Secret

```go
func mapErr(err error) *domain.Error {
    if errors.Is(err, ErrNotConnected) { return sdk.ConnectionError(domain.CodeChannelLost, "lost") }
    if errors.Is(err, ErrOffline)      { return sdk.DeviceStateError(domain.CodeDeviceOffline, "offline") }
    return sdk.ProtocolError(domain.CodeInternal, err.Error(), err.Error()) // raw 只进 details
}
```

- 密码/token 一律用 `security.SecretRef` 引用，`SecretStore.Resolve` 用时才取，日志前 `Redact`。

## 第 5 步：在 runtime 注册（唯一允许改的现有代码）

```go
// runtime/bootstrap.go
import "<module>/plugin/<proto>"
...
p := <proto>.New(...)
_ = plugins.Register(p); _ = plugins.Load(<proto>.PluginID); plugins.Ready(<proto>.PluginID)
```

## 第 6 步：合同测试

```go
// tests/contract/plugin_contract_test.go
func Test<Proto>PluginContract(t *testing.T) {
    farm := fake.NewFarm(); farm.Add(fake.NewDevice(...))
    AssertPluginContract(t, <proto>.New(farm), farm, Case{
        EndpointType: domain.Endpoint<YOURTYPE>, Locator: "...", ChannelType: "<proto>", SampleCapability: domain.CapabilityInfoGet,
    })
}
```

## 验证

```bash
export PATH=$HOME/.local/go/bin:$HOME/.local/protoc/bin:$PATH
go build ./... && go vet ./... && go test ./...
# 特别确认架构门禁：go test ./tests/arch/
```

## 红线检查清单（提交前逐条过）

- [ ] 没改 `core/` `sdk/`（除 runtime 注册）
- [ ] 没在 `core/`/`sdk/` 出现协议名字面量
- [ ] 插件只 import `domain` / `fake` / `plugin/sdk`
- [ ] `Manifest.ID` 稳定唯一，`APIVersion` 非空，`TrustLevel` 合法
- [ ] `Probe` 无副作用、可超时、返回 `Confidence` 与 `IdentityHints`
- [ ] 错误全部归一化为 `domain.Error`，raw 文本只进 `details`
- [ ] 有 `AssertPluginContract` 合同测试且 `go test ./...` 全绿
