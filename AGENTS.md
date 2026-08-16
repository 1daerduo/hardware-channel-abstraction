# AGENTS.md — 本项目 AI 编程代理须知

> 本文件是给 AI 编程助手（Claude Code、Codex、Cursor 等）的项目级指令。
> 任何对代码的修改都必须遵守下面的「最高原则」与「红线」。

## 项目是什么

**Embedded Loop Channel Abstraction（嵌入式 Loop 通道抽象层）**：上层通过**统一 API**
访问异构嵌入式设备，把 ADB / UART / JTAG / TCP / MCP / BLE 等协议差异隔离在
**Channel Plugin SPI** 之后。消费者只知道「访问某台设备」，不关心它背后是串口、
ADB 还是网络接入。

核心抽象链：**Endpoint → Channel → Capability → Operation**。

## 最高原则（不可违背）

1. **协议差异只进 Plugin**：`core` / `sdk` / `domain` 不得出现协议名（adb/uart/jtag/mcp/fastboot）、
   不得出现 `if protocol == X` 特判。
2. **依赖方向单向**：`domain ← core ← sdk/runtime`；`plugin → plugin/sdk`。
3. **上层只面对抽象**：消费方只面向 `sdk.ConnectivityAPI` + 抽象链，不感知具体协议/传输。
4. **新增协议 = 新增 Plugin + 合同测试**，不修改 core/sdk/domain。

## 目录地图

| 目录 | 职责 | 能否被新代码依赖 |
|---|---|---|
| `domain/` | 稳定领域概念（Device/Endpoint/Channel/Capability/Operation/...） | 仅标准库 |
| `core/` | 编排：discovery/resolver/session/resource/operation/event/artifact/recovery/security | 不得依赖具体插件 |
| `plugin/sdk` | Plugin SPI 契约（Plugin/Manifest/ConsoleDevice/Stream/Canceller） | 被插件依赖 |
| `plugin/adb` `plugin/uart` `plugin/mcp` | 具体协议实现 | 只依赖 domain/fake/plugin-sdk |
| `transport/serial` `transport/grpc` | 真实传输（串口、gRPC） | — |
| `sdk/` | 统一 API（`ConnectivityAPI` 接口 + Client） | 不得依赖具体插件 |
| `runtime/` | 装配根（Bootstrap） | **唯一允许 import 具体插件的地方** |
| `fake/` | 内存设备模拟器 | 仅依赖 domain |

## 新增一种协议/连接：调用 skill

当任务是「新增/接入一种协议或连接方式」（TCP、BLE、JTAG、Vendor RPC…）时，
**先加载并遵循 skill `add-channel-protocol`**（见 `.claude/skills/add-channel-protocol/SKILL.md`）。
它会引导你：先评估连接形态 → 再按 SPI 实现 → 注册 → 合同测试。

## 验证命令

```bash
export PATH=$HOME/.local/go/bin:$HOME/.local/protoc/bin:$PATH
go build ./...     # 编译
go vet ./...       # 静态检查
go test ./...      # 全量测试（含架构门禁）
```

## 红线（架构门禁 `tests/arch` 会直接拦截）

- `core`/`sdk` 源码出现 `"adb"` / `"uart"` / `"jtag"` / `"mcp"` / `"fastboot"` 字面量。
- `core` 依赖 `plugin/adb|uart|mcp` 或 `fake`。
- `plugin/*` 依赖 `core`。
- `fake` 依赖 domain 之外的业务包。
- `sdk` 依赖具体插件。

## 关键设计不变量

- **UNKNOWN 是一等状态**：结果未知 ≠ 成功/失败。
- **deny-by-default**：未授权能力 `DENY`；HIGH/CRITICAL 需审批。
- **Retry 是 Policy**：重试/恢复由 Core 控制，Plugin 不自行无限重试。
- **错误结构化**：统一 `code/category/retryable/recoverable/severity`，raw 文本只进 `details`。
- **不承诺 exactly-once**：用 Operation Identity + 幂等 + 观测 + 后置校验达到 effectively-once。
- **Observe-first**：断链先观测，不盲目重放。

## 更多上下文

- 架构与实现总结：`SUMMARY.md`
- 快速上手指南：`README.md`
