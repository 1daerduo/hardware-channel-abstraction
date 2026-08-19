# Channel Plugin SPI 规范

> 版本：v1.0（`api_version = "1.0"`）
> 面向：要接入新协议 / 新设备的第三方工程师
> 配套：`plugin/_template/`（从零开始的插件模板）、`plugin/sdk`（Go SDK）、`tests/contract`（可执行验收）

本规范是「受治理的硬件通道抽象层」对**接入方**的唯一契约。接入方只要满足本规范，
就能让上层（统一 API / Agent / Loop / CLI）通过抽象使用它的设备，而框架层零改动、零迁就。

---

## 1. 定位与边界

```
────────────────────────────────────────────────────
 SPI 之上：框架层（domain / core / sdk / runtime）
     —— 只面对抽象，永不改动，永不迁就具体协议
────────────────────────────────────────────────────
 SPI 之下：接入方 = 一个 sdk.Plugin 实现（一个 Go 包）
     —— 协议差异全部隔离在这里，框架不关心内部
────────────────────────────────────────────────────
```

- **框架层**定义契约，**接入方**实现契约。
- 接入方内部是 ADB over USB/WIFI、CLI 反射、Vendor RPC、还是别的，**框架一概不关心**。
- 接入方的唯一义务：对外暴露一个满足本规范的 `sdk.Plugin`。

**一句话准入**：实现 SPI → 通过合同测试 `AssertPluginContract` → 在 `runtime` 注册一行 → 上层统一抽象使用。

---

## 2. 核心概念（10 秒理解）

| 概念 | 回答的问题 | 类型 |
|---|---|---|
| `Endpoint` | 「它在哪里可达」 | 可发现的入口（串口路径 / USB 路径 / 网络地址） |
| `Channel` | 「一条已建立的连接」 | Endpoint 打开后的运行时实例 |
| `Capability` | 「设备能做什么」 | 自描述能力（name + JSON Schema + risk） |
| `Operation` | 「一次执行」 | 能力的一次调用，有状态机 |

链路：**Endpoint → Channel → Capability → Operation**。
上层只引用 `Capability`（如 `device.info.get`），**从不引用协议命令**。

---

## 3. SPI 契约：必选方法（9 个）

接口定义见 `plugin/sdk/plugin.go`。以下每个方法都附「必须满足的不变量」。

### 3.1 `Info() Manifest`
返回插件的声明式元数据。**稳定、只读**。字段规则见 §5。

### 3.2 `Probe(ctx, endpoint) (ProbeResult, error)`
判断「这个 Endpoint 是否属于本插件」。

**不变量**：
- **轻量、无副作用**：不得建立连接、不得改变设备状态。
- 不匹配时返回 `ProbeResult{Match: false}` 和 `nil` error，**不要报错**。
- 匹配时返回 `Confidence > 0` 和 `IdentityHints`（供设备身份关联/去重）。
- `Cost` 表示该通道的独占成本（普通连接 ~10，独占调试口 ~30），供选路排序。

### 3.3 `Capabilities(channel) []Capability`
返回该通道暴露的能力集（**带 JSON Schema 的自描述**，见 §7）。

**不变量**：
- 签名**带 channel 参数**——支持按设备动态返回能力（例如设备侧 CLI 反射）。静态能力也走这个入口。
- 返回的切片不得被调用方原地修改（内部拷贝后返回）。

### 3.4 `Open(ctx, channel, session) error`
校验 Endpoint 并建立通道。成功后置 `channel.State = READY`、`channel.Healthy = true`。
失败必须返回归一化错误（§8），不得裸返回底层库错误。

### 3.5 `Close(ctx, channel) error`
拆除通道。成功后置 `channel.State = CLOSED`、`channel.Healthy = false`。

### 3.6 `Invoke(ctx, channel, req) (*InvokeResult, error)`
统一能力 → 协议动作。**这是核心方法**。

**不变量**：
- 只处理 `req.Capability` 声明过的能力；未声明/不支持的能力返回 `UNSUPPORTED_CAPABILITY` + `CategoryValidation`。
- 返回 typed 结果（`Output` + `Evidence` + `Artifacts`），**raw 协议文本不得跨越边界**——只能进 Evidence/Artifact 或错误 Details。
- 设备离线 → `CategoryDeviceState`；断链 → `CategoryConnection`；参数非法 → `CategoryValidation`。

### 3.7 `Health(ctx, channel) error`
快速、低副作用的 liveness 检查。**不得改变设备状态**。
健康则置 `channel.Healthy = true`；离线则置 false 并返回 `DeviceStateError`。

### 3.8 `Observe(ctx, channel, req) (*Observation, error)`
**只读**观测设备当前状态，用于「Observe-first」对账（中断后判断 SUCCESS / retry / UNKNOWN）。
**不得改变设备状态**。返回 `Online` + 后置条件 `Facts`（如 `flash.version` → `2.0.0`）。

### 3.9 `Recover(ctx, channel, reason) error`
协议级恢复动作。**只在 Core 的 Recovery Policy 允许时才会被调用**，插件不自行无限重试。
`reason` 是 Core 传入的恢复阶梯（如 `"reconnect"`、`"device_recovery"`）。

---

## 4. 可选接口（按需实现，Core 用类型断言探测）

| 接口 | 方法 | 用途 |
|---|---|---|
| `sdk.Streamer` | `Stream(ctx, ch, req) (Stream, error)` | 持续数据流（console/log/trace） |
| `sdk.Canceller` | `Cancel(ctx, ch, opID) error` | 协作式取消在途操作；不能取消则返回 `NOT_CANCELLABLE` |
| `sdk.ConsoleDevice` | 见 `plugin/sdk/device.go` | **字节流类**设备的传输抽象（串口/TCP/SSH 控制台） |

> **连接形态 → 实现路径**（判断口诀）：
> - 设备能「敲命令回车看回显」→ **字节流控制台**：实现 `sdk.ConsoleDevice`，包装进 `transport/console`。
> - 设备只能「发结构化请求等响应」→ **直接实现 `sdk.Plugin`**，底层用对应库。
> - 远程服务（MCP/云端 API）→ 参考 `plugin/mcp` 的 tool→capability 映射。

---

## 5. Manifest

定义见 `plugin/sdk/manifest.go`。

| 字段 | 类型 | 要求 |
|---|---|---|
| `ID` | string | **稳定、全局唯一**（约定 `protocol.<name>`），版本变化**永不改名** |
| `Version` | string | 插件自身版本（语义化，如 `1.0.0`） |
| `APIVersion` | string | 本 SPI 契约版本，当前固定 `"1.0"` |
| `Protocol` | string | 协议名（`adb` / `uart` / `jtag` / `modbus` …） |
| `Capabilities` | []CapabilityName | 至少 1 个 |
| `Transports` | []string | 传输形态（`usb` / `tcp-ip` / `serial` / `debug-probe` …） |
| `RecoveryActions` | []string | 支持的恢复动作（`reconnect` / `rediscover` …） |
| `TrustLevel` | TrustLevel | `TRUSTED` / `VERIFIED` / `UNTRUSTED` |

`Manifest.Validate()` 已强制：ID 非空、APIVersion 非空、至少 1 个能力、TrustLevel 合法。

---

## 6. 数据结构（`plugin/sdk/results.go`）

**`ProbeResult`**：`Match` / `Confidence` / `IdentityHints` / `ChannelType` / `Cost`

**`InvokeRequest`**：`Capability` / `Target` / `Parameters`(map[string]string) / `SessionID` / `OperationID`

**`InvokeResult`**：`Output`(string) / `Evidence`([]domain.Evidence) / `Artifacts`([]domain.Artifact)

**`Observation`**：`Online`(bool) / `State`(string) / `Facts`(map[string]string)

---

## 7. Capability 自描述

定义见 `domain/capability.go`。这是本架构对接 LLM/Agent 的桥梁——
`{name, description, input_schema}` 就是 function-calling 的 tool 定义（`ToolDefinition()` 直接生成）。

| 字段 | 要求 |
|---|---|
| `Name` | 优先复用 `domain.Capability*` 常量；协议独有能力才新增常量 |
| `Description` | 一句话说明做什么（Agent 据此选 tool） |
| `InputSchema` / `OutputSchema` | **JSON Schema**（用 `domain.ObjectSchema / StringSchema / StringEnumSchema` 构建） |
| `RiskLevel` | `LOW` / `MEDIUM` / `HIGH` / `CRITICAL`（驱动审批，不是权限） |
| `Idempotent` | 是否幂等（影响重放策略） |
| `ResourceRequirements` | 所需资源**类型**（`device` / `flash` / `debug` …） |

**可复用标准能力**（`domain/capability.go` 已定义）：
`device.info.get` / `device.reboot` / `device.flash` / `device.execute` / `device.file.read` /
`device.log` / `device.console` / `device.reset` / `debug.*` / `modbus.*`

---

## 8. 错误归一化（硬性要求）

**所有错误必须返回 `*domain.Error`**，raw 协议文本只进 `Details`。自动化依赖稳定
`code/category` 做分支，**绝不 parse 消息文本**。

构造用 `plugin/sdk` 提供的 4 个构造函数：

| 构造函数 | 用途 |
|---|---|
| `sdk.Error(code, cat, msg)` | 通用归一化 |
| `sdk.ProtocolError(code, msg, raw)` | 协议失败，raw 只进 details |
| `sdk.DeviceStateError(code, msg)` | 设备侧状态（离线等） |
| `sdk.ConnectionError(code, msg)` | 传输断链（自动标 `recoverable=true`） |

**Category 映射表**（什么时候用什么）：

| 场景 | Category | 常用 Code |
|---|---|---|
| 参数非法 / 能力不支持 | `VALIDATION` | `INVALID_INPUT` / `UNSUPPORTED_CAPABILITY` |
| 设备离线 / 状态不对 | `DEVICE_STATE` | `DEVICE_OFFLINE` |
| 通道丢失 / 断链 | `CONNECTION` | `CHANNEL_LOST` |
| 底层协议失败 | `PROTOCOL` | `INTERNAL` |

完整 `ErrorCategory` 与 `Code` 常量见 `domain/error.go`。

---

## 9. Channel 生命周期

定义见 `domain/channel.go`。**状态由 Core 管理**，插件只在 `Open/Close/Recover` 里
写 `READY` / `CLOSED` / `RECONNECTING`。

```
UNRESOLVED → RESOLVING → OPENING → READY ⇄ DEGRADED → RECONNECTING → READY
                                        ↓
                                     CLOSED / FAILED
```

---

## 10. 依赖方向红线（架构门禁强制）

| 允许 import | 禁止 import |
|---|---|
| `domain` | `core`（任何子包） |
| `plugin/sdk` | 其它 `plugin/*`（插件间不得互相依赖） |
| `fake`（仅测试用） | 业务侧 `sdk`（顶层） |

`runtime` 是**唯一**允许 import 具体插件的地方。违反会被 `go test ./tests/arch/` 拦截。

---

## 11. 接入流程（7 步）

1. **复制模板** `plugin/_template/` → `plugin/<your-proto>/`。
2. **全局替换**占位符 `template` → 你的协议名；改 `PluginID` / `ChannelType`。
3. **实现能力**：在 `capabilities.go` 声明能力（复用标准能力或新增），在 `invoke.go` 写映射。
4. **错误归一化**：所有底层错误经 §8 的构造函数转成 `*domain.Error`。
5. **注册一行**（`runtime/bootstrap.go`，唯一改点）：
   ```go
   p := yourproto.New(...)
   _ = plugins.Register(p)
   _ = plugins.Load(yourproto.PluginID)
   plugins.Ready(yourproto.PluginID)
   ```
6. **写合同测试**（`tests/contract/plugin_contract_test.go`）：
   ```go
   AssertPluginContract(t, yourproto.New(farm), farm, Case{ ... })
   ```
7. **验证**：`go build ./... && go vet ./... && go test ./...` 全绿。

---

## 12. 验收 = 合同测试

`AssertPluginContract`（`tests/contract/plugin_contract_test.go`）是**可执行准入标准**，覆盖：

1. Manifest 合法（稳定 ID / api_version / 能力 / trust）。
2. Probe 正确匹配已知端点、拒绝未知 locator、拒绝外来 EndpointType。
3. 能力元数据完整（name / version / risk 非空）。
4. Open → READY+Healthy；Close → CLOSED。
5. Invoke 返回 typed 结果 + evidence；不支持能力 → `CategoryValidation`。
6. 设备离线 → `CategoryDeviceState`（不是 raw 文本）。
7. Recover 后通道回到 READY。

**过不了合同测试 = 不满足 SPI，不予接收。**

---

## 13. 兼容性承诺

- 本规范对应 `api_version = "1.0"`。
- **稳定性承诺**：`Plugin` 接口的 9 个必选方法签名不破坏性变更；破坏性变更必须升 `api_version`。
- **扩展方式**：新增可选接口（`Streamer` / `Canceller`）通过类型断言探测，不影响既有插件。
- 插件升级（`Version` 变化）不得改名 `ID`；`ID` 是身份，`Version` 是版本。

---

## 14. 参考实现

| 插件 | 协议 | 演示什么 |
|---|---|---|
| `plugin/adb` | USB/ADB | **最小完整示例**：静态能力 + 证据/产物 + 恢复 |
| `plugin/uart` | 串口 | 字节流控制台（`ConsoleDevice` 复用 fake 与真实串口） |
| `plugin/tcp` | TCP | 字节流控制台（真实 TCP） |
| `plugin/mcp` | 远程服务 | tool→capability 映射（结构化协议） |
| `plugin/jtag` | JTAG/SWD | 独占调试资源 + HIGH/CRITICAL 风险 |
| `plugin/modbus` | Modbus TCP | 结构化寄存器协议（无第三方库） |

**模板**：`plugin/_template/` —— 从零开始照抄的最小骨架（不参与构建，接入时复制改名）。
