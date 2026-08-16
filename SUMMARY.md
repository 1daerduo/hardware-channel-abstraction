# Embedded Loop Channel Abstraction — 实现总结

> 基于 `Embedded_Loop_Channel_Abstraction_Final_Baseline_HTML`（文档 01~18）的 Go 实现。
> 本文档回答：我们实现了什么、达成了什么架构、以及如何拓展新协议。

## 1. 一句话定位

上层（Embedded Loop / Agent / CLI / SDK）通过**统一 API** 访问异构嵌入式设备，
把 ADB / UART / JTAG / TCP / MCP / BLE 等协议差异隔离在 **Channel Plugin SPI** 之后。
消费者只知道「访问某台设备」，不关心它背后是串口、ADB 还是网络接入。

核心抽象链：**Endpoint → Channel → Capability → Operation**；
Session / Lock / Policy / Health / Event / Artifact / Recovery 提供工程治理。

## 2. 达成的架构

```
Consumer (Loop / Agent / CLI)          ← 只面向 ConnectivityAPI（抽象）
        │
  Unified API (sdk.ConnectivityAPI)
        │   ├─ 进程内:   sdk.Client
        │   └─ 远程:     transport/grpc.Client      ← 传输可插拔
        ↓
  Connectivity Core (core/*)           ← 编排，协议无关
        ↓
  Channel Plugin SPI (plugin/sdk)      ← 协议隔离层
        │   ├─ plugin/adb   (USB/ADB)
        │   ├─ plugin/uart  (串口控制台)
        │   └─ plugin/mcp   (远程服务)
        ↓
  Transport (fake 模拟 / transport/serial 真实串口)
        ↓
  Device / Endpoint (domain)
```

### 依赖方向（已固化为 CI 门禁）

| 规则 | 门禁 |
|---|---|
| `domain` 仅依赖标准库 | `TestArchitectureImportRules` |
| `core` 不依赖具体插件、不出现协议名 | 同上 + `TestCoreHasNoProtocolSpecials` |
| `plugin/*` 不依赖 `core` | `TestArchitectureImportRules` |
| `fake` 仅依赖 `domain` | `TestArchitectureImportRules` |
| `sdk` 不依赖具体插件 | `TestArchitectureImportRules` |

## 3. 已实现能力总览

| 能力 | 落地 | 文档 |
|---|---|---|
| 统一抽象链 | Endpoint→Channel→Capability→Operation + 全领域模型 | 02 |
| 统一 API | `sdk.ConnectivityAPI` 接口（进程内 + gRPC 远程双实现） | 03 |
| Plugin SPI | Probe/Open/Invoke/Health/Observe/Recover/Cancel/Stream | 04,12,16 |
| 三协议接入 | ADB / UART / MCP，Core 零改动 + Contract Test | 12,24 |
| 发现 + 热插拔 | Discovery/Refresh/Watch + 身份关联 + 冲突隔离(Quarantine) | 05 |
| 能力选路 | Capability→Channel 确定性排序 + override | 05,12 |
| 可靠执行 | 状态机 + 幂等 + 异步(Start/Wait) + 取消(Cancel) | 06 |
| 并发治理 | Session / Lock(SHARED/EXCLUSIVE) / Lease / 死锁防护 | 07 |
| 事件与产物 | EventBus(序列/游标) + Artifact(checksum/verify) + Evidence + 流式 | 08 |
| 恢复 | L0~L6 阶梯 + Observe-first + State Reconciliation + Budget/Backoff | 09 |
| 安全 | AuthN + deny-by-default + 资源范围 + 密钥(SecretRef/Redact) + 审批 + 审计 | 10 |
| 传输网络化 | gRPC ConnectivityService（薄适配，共享同一接口） | 03,14 |
| 真机验证 | i.MX6ULL U-Boot 统一 API 执行 + 流式控制台 | 12 |
| 测试门禁 | Contract / Unit / Architecture / Mutation | 17 |

## 4. 关键设计不变量

- **UNKNOWN 是一等状态**：`OperationUnknown` 是终态且不是成功。
- **deny-by-default**：未授权能力 `DENY`；HIGH/CRITICAL 需审批。
- **Retry 是 Policy**：业务重试由 Core 控制，恢复受预算约束，Plugin 不自行无限重试。
- **错误结构化**：统一 `code/category/retryable/recoverable/severity`，raw 文本只进 `details`。
- **不承诺 exactly-once**：用 Operation Identity + 幂等 + 状态观测 + 后置校验达到 effectively-once。
- **Observe-first**：断链先观测设备状态与后置条件，不盲目重放。

## 5. 如何拓展

### 5.1 新增一种「协议」（如 JTAG / TCP / 新 Vendor）

只需 6 步，只有第 4 步动到现有非测试代码：

1. 建 `plugin/<proto>/`，实现 `sdk.Plugin`（Manifest/Probe/Open/Invoke/Health/Observe/Recover + 可选 Cancel/Stream）。
2. 定义 Capability 映射（统一能力 → 协议动作）。
3. 错误归一化 + Secret 走 `SecretRef`。
4. 在 `runtime/bootstrap.go` 注册该 Plugin（**唯一改点**）。
5. 写 `AssertPluginContract` 合同测试。
6. `go test ./...` 全绿（架构门禁校验未改 core/sdk）。

### 5.2 新增一种「传输」（真实设备替代 fake）

实现 `plugin/sdk.ConsoleDevice`（或 `StreamProvider`），在 runtime 的 resolver/scanner 登记。
Plugin 逻辑一行不改——`plugin/uart` 同时驱动 `fake.Device` 与真实串口就是证明。

### 5.3 新增一个「Capability」

在对应 Plugin 的 `Capabilities()` 加描述 + `Invoke` 加 case，不改 Loop/统一 API/领域模型。

## 6. 状态与边界

- **已完成**：除 Loop Runtime（消费方）外，文档 01~18 的设计已基本落地。
- **MVP 明确不做**（文档 18 §43）：多租户、复杂审批流、全量 Event Replay、动态热升级。
- **Beta 待做**：真实 ADB/MCP 客户端替换 fake、Artifact Retention、完整 Approval 工作流、真实设备 E2E 全矩阵。
- **Production**：Plugin 沙箱、审计不可变存储、HA。

## 7. 验证

- `go build ./...`、`go vet ./...`、`go test ./...` 全绿。
- 78 个 Go 文件、51 个测试，覆盖 Contract / Unit / Architecture / Mutation。
- 真机：i.MX6ULL EVK @ U-Boot 2016.03，`device.execute` + `device.log` 流式控制台实测通过。
