# Embedded Loop Channel Abstraction — 实现总结

> 基于 `Embedded_Loop_Channel_Abstraction_Final_Baseline_HTML`（文档 01~18）的 Go 实现。
> 本文档回答：我们实现了什么、达成了什么架构、以及如何拓展新协议。

## 1. 一句话定位

上层（Embedded Loop / Agent / CLI / SDK）通过**统一 API** 访问异构嵌入式设备，
把 ADB / UART / JTAG / TCP / MCP / BLE 等协议差异隔离在 **Channel Plugin SPI** 之后。
消费者只知道「访问某台设备」，不关心它背后是串口、ADB 还是网络接入。

核心抽象链：**Endpoint → Channel → Capability → Operation**；
Session / Lock / Policy / Health / Event / Artifact / Recovery 提供工程治理。

## 2. 产品定位与落地生态

### 2.1 本质：受治理的设备能力层

不是「MCP server」，不是「协议转换器」，而是介于物理设备与「任何想控制它的人/程序」之间的
**受治理的设备能力层（Governed Device Capability Layer）**。三条 DNA 决定它的归属：

1. **设备接入**：串口 / ADB / JTAG / TCP / BLE / CAN 可插拔。
2. **可靠性**：恢复 L0~L6、UNKNOWN 一等状态、对账、预算/退避。
3. **治理**：deny-by-default、审批、审计、锁/会话、资源范围。

这三条指向的不是「给 LLM 发工具」，而是「一群设备要被可信地、批量地、可恢复地操作」。

### 2.2 落地生态（按 DNA 匹配度排序）

| 优先级 | 生态 | 场景 | 我们提供的价值 |
|---|---|---|---|
| **主战场** | 嵌入式设备农场 / 实验室 CI 编排 | 几十上百块板子批量刷写/重启/测试/采日志/自动恢复 | 多 Endpoint 自动选路、恢复预算防 Reset Storm、UNKNOWN 对账（文档 01~18 原意） |
| **增长** | AI Agent 操作真实设备的执行层 | Agent 安全地「动手」操作设备 | 审批 + 审计 + deny-by-default 是「让 Agent 操作设备」缺的护城河 |
| **横向** | IoT / 边缘设备统一接入中间件 | 任何上层系统通过统一 Capability 接入设备 | 抽象 + 便携拓展 |

### 2.3 MCP 的角色：一个「头」，不是本体

MCP 在本架构里有两个位置，都很自然：

| 位置 | 角色 | 状态 |
|---|---|---|
| 下方（协议） | 只暴露 MCP 的设备 → `plugin/mcp` 当普通协议接入 | 已实现 |
| 上方（头） | 把设备能力暴露给 MCP 客户端（Claude/Cursor） | 薄适配（未做） |

MCP 是「AI 生态的通用插头」，接上它就能低成本触达 Claude / Cursor / 各类 agent；
但产品本体是「设备能力层」，不是 MCP server。

### 2.4 一句话结论

> 产品定位：**受治理的设备能力层**。
> 主生态：**嵌入式设备农场 / 实验室 CI 编排**。
> 增长生态：**AI Agent 操作真实设备的执行层（治理是护城河）**。
> MCP：**接入 AI 生态的一个标准「头」，不是产品本体**。

换言之：我们在做的不是「MCP 工具」，而是「AI 和自动化系统的物理世界执行底座」——
设备农场 CI 是今天能落地的价值，Agent 执行层是明天的增量，MCP 是两者共用的插头。

## 3. 达成的架构

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

## 4. 已实现能力总览

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

## 5. 关键设计不变量

- **UNKNOWN 是一等状态**：`OperationUnknown` 是终态且不是成功。
- **deny-by-default**：未授权能力 `DENY`；HIGH/CRITICAL 需审批。
- **Retry 是 Policy**：业务重试由 Core 控制，恢复受预算约束，Plugin 不自行无限重试。
- **错误结构化**：统一 `code/category/retryable/recoverable/severity`，raw 文本只进 `details`。
- **不承诺 exactly-once**：用 Operation Identity + 幂等 + 状态观测 + 后置校验达到 effectively-once。
- **Observe-first**：断链先观测设备状态与后置条件，不盲目重放。

## 6. 如何拓展

### 6.1 新增一种「协议」（如 JTAG / TCP / 新 Vendor）

只需 6 步，只有第 4 步动到现有非测试代码：

1. 建 `plugin/<proto>/`，实现 `sdk.Plugin`（Manifest/Probe/Open/Invoke/Health/Observe/Recover + 可选 Cancel/Stream）。
2. 定义 Capability 映射（统一能力 → 协议动作）。
3. 错误归一化 + Secret 走 `SecretRef`。
4. 在 `runtime/bootstrap.go` 注册该 Plugin（**唯一改点**）。
5. 写 `AssertPluginContract` 合同测试。
6. `go test ./...` 全绿（架构门禁校验未改 core/sdk）。

### 6.2 新增一种「传输」（真实设备替代 fake）

实现 `plugin/sdk.ConsoleDevice`（或 `StreamProvider`），在 runtime 的 resolver/scanner 登记。
Plugin 逻辑一行不改——`plugin/uart` 同时驱动 `fake.Device` 与真实串口就是证明。

### 6.3 新增一个「Capability」

在对应 Plugin 的 `Capabilities()` 加描述 + `Invoke` 加 case，不改 Loop/统一 API/领域模型。

## 7. 与 LLM / Agent 生态的映射

这套抽象与 AI 生态是**同构**的，而非勉强兼容：

| 本抽象 | LLM / Agent 生态 |
|---|---|
| `Capability`（name + version + risk + 描述 + schema） | **Tool**（工具定义） |
| `Capability.input_schema / output_schema` | **function calling 的参数/返回 JSON Schema** |
| `Operation`（一次执行，输入→输出，状态机） | **tool call / agent 的 action** |
| `ConnectivityAPI`（Discover / ListCapabilities / Execute） | **MCP / A2A 的统一调用面** |
| `Plugin SPI`（接入侧） | **MCP server / connector / tool provider** |
| `Resolver`（capability→channel 自动选路） | **tool router** |
| deny-by-default + risk/审批 + 审计 | **AI 安全：最小权限 + human-in-the-loop** |
| Event / Artifact / Evidence | **可观测性 / artifact / 审计** |

双向契合：`plugin/mcp` 已把 MCP 当作与 ADB/UART 平级的协议接入（向下）；`ListCapabilities + Execute`
天然就是 LLM 的 tool 发现 + 调用（向上，Loop Runtime 同理是消费方，不碰核心）。

要真正进 AI 生态，只需消费侧薄适配：把每个 `Capability` 翻译成
`{name, description, input_schema}` 的 tool 定义，把 `Execute` 包装成 tool invoke——
纯增量，落在「抽象 + 便携拓展」红线内。

## 8. 状态与边界

- **已完成**：除 Loop Runtime（消费方）外，文档 01~18 的设计已基本落地。
- **MVP 明确不做**（文档 18 §43）：多租户、复杂审批流、全量 Event Replay、动态热升级。
- **Beta 待做**：真实 ADB/MCP 客户端替换 fake、Artifact Retention、完整 Approval 工作流、真实设备 E2E 全矩阵。
- **Production**：Plugin 沙箱、审计不可变存储、HA。

## 9. 验证

- `go build ./...`、`go vet ./...`、`go test ./...` 全绿。
- 78 个 Go 文件、51 个测试，覆盖 Contract / Unit / Architecture / Mutation。
- 真机：i.MX6ULL EVK @ U-Boot 2016.03，`device.execute` + `device.log` 流式控制台实测通过。
