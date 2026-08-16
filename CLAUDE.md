# CLAUDE.md

本文件是 Claude Code 的项目级指令。完整规则见 `AGENTS.md`（@AGENTS.md）。

## 一句话定位

Embedded Loop Channel Abstraction：上层通过**统一 API** 访问异构设备，协议差异隔离在
**Channel Plugin SPI** 之后。核心抽象链：**Endpoint → Channel → Capability → Operation**。

## 必须遵守的红线（违反会被 `tests/arch` 架构门禁拦截）

1. **协议差异只进 Plugin**：`core`/`sdk`/`domain` 不得出现协议名（adb/uart/jtag/mcp/fastboot）、
   不得出现 `if protocol == X`。
2. **依赖方向单向**：`domain ← core ← sdk/runtime`；`plugin → plugin/sdk`。
3. **新增协议 = 新增 Plugin + 合同测试**，不修改 core/sdk/domain。
4. `runtime/` 是唯一允许 import 具体插件（plugin/adb|uart|mcp）的地方。

## 新增一种协议/连接时

先加载并遵循 skill **`add-channel-protocol`**（`.claude/skills/add-channel-protocol/SKILL.md`）：
评估连接形态 → 实现 SPI → 注册 → 合同测试。

## 验证

```bash
export PATH=$HOME/.local/go/bin:$HOME/.local/protoc/bin:$PATH
go build ./... && go vet ./... && go test ./...
```

@AGENTS.md
