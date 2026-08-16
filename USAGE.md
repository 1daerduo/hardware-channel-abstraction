# USAGE.md — 快速上手

> 如何使用这套「受治理的设备能力层」：直接控制设备、起中央服务、批量刷写、任务队列。
> 架构与实现见 `SUMMARY.md`；新增协议见 `AGENTS.md` + skill `add-channel-protocol`。

## 0. 构建

```bash
export PATH=$HOME/.local/go/bin:$PATH
cd embedded-loop-channel
go build -o elc ./cmd/elc        # 得到 elc 命令行
# 或临时跑：go run ./cmd/elc ...
```

## 1. 三种使用模式

| 模式 | 形态 | 适合 | 中央服务落点 |
|---|---|---|---|
| **直接模式** | `elc` 进程内嵌抽象层，直接开设备发指令 | 单机、交互式调试 | 无 |
| **服务模式** | `elc serve` 起中央服务，客户端 gRPC 连 | 多端、批量、农场 | 持有设备的机器 |
| **库模式** | Go 代码 `import sdk`，面向 `ConnectivityAPI` | Loop/Agent/内嵌 | 进程内 |

> 核心原则：**谁持有设备的串口/TCP 连接，谁做中央服务**；客户端落在任何能网络连到它的地方。

## 2. 直接模式（单机控制）

```bash
# 发现设备（无 flags 默认带一台内置 fake 设备，方便体验）
elc devices

# 通过真实串口给 U-Boot 发指令
elc exec --serial /dev/ttyUSB0 /dev/ttyUSB0 device.execute command="version"
elc exec --serial /dev/ttyUSB0 /dev/ttyUSB0 device.execute command="mmc list"

# 通过 TCP 设备发指令
elc exec --tcp 127.0.0.1:58732 <设备> device.execute command="echo hi"
```

设备引用 = 设备 ID 或 serial（支持前缀）。`device.execute` = 发命令 + 读回显。

## 3. 服务模式（中央服务 + 远程客户端）

```bash
# 1) 起中央服务（持有设备 + 农场调度器，同时暴露两个 gRPC service）
elc serve --listen :8080 --serial /dev/ttyUSB0

# 2) 任何客户端通过 gRPC 连它
elc devices --grpc localhost:8080
elc exec    --grpc localhost:8080 /dev/ttyUSB0 device.execute command="version"

# 3) 农场任务队列 / 设备池（FarmService）
elc submit --grpc localhost:8080 device.flash partition=boot version=2.0.0 --priority 5
elc tasks  --grpc localhost:8080
elc task   --grpc localhost:8080 task-0
elc pool   --grpc localhost:8080
```

## 4. 库模式（Go 代码嵌入）

```go
import "github.com/1daerduo/hardware-channel-abstraction/sdk"

// api 是 sdk.ConnectivityAPI：进程内 Client 或远程 grpc.Client 都实现它。
var api sdk.ConnectivityAPI

devices, _ := api.Discover(ctx)                 // 发现设备
caps, _ := api.ListCapabilities(devices[0].ID)  // 设备能干什么
sess, _ := api.CreateSession("agent", devices[0].ID, time.Minute)
res, err := api.Execute(ctx, domain.OperationRequest{
    Capability: domain.CapabilityFlash,
    Target:     devices[0].ID,
    SessionID:  sess.ID,
    Parameters: map[string]string{"partition": "boot", "version": "2.0.0"},
})
// res.State / res.Output / res.EvidenceRefs / res.ArtifactRefs
```

## 5. CLI 命令速查

| 命令 | 作用 |
|---|---|
| `elc devices [--json]` | 发现并列出设备 |
| `elc caps <device> [--json]` | 列出能力（含描述 + JSON Schema，即 LLM 工具定义） |
| `elc exec <device> <capability> [k=v...] [--json]` | 执行一个能力 |
| `elc batch <capability> [k=v...] [--devices a,b] [--concurrency N]` | 批量执行（多台设备） |
| `elc queue <capability> [k=v...]` | 进程内任务队列演示（提交+等待+设备池） |
| `elc stream <device> <capability>` | 流式控制台（进程内） |
| `elc serve [--listen :8080] [--serial ...] [--tcp ...]` | 起中央服务 |
| `elc submit <capability> [k=v...] [--priority N] [--devices a,b]` | 提交任务到中央调度器 |
| `elc tasks` / `elc task <id>` | 任务列表 / 状态 |
| `elc pool` | 设备池状态（busy/idle + 最后结果） |

后端 flags（任意子命令，可放在位置参数前后）：`--grpc <addr>` / `--serial <path> --baud <n>` / `--tcp <addr>`。

## 6. 接入你的设备

| 设备类型 | 方式 |
|---|---|
| 串口（CH340/FTDI…） | `--serial /dev/ttyUSB0 --baud 115200`（UART 插件） |
| TCP 直连 | `--tcp <addr>`（TCP 插件，字节流控制台） |
| 远程服务/网络 | `--grpc <addr>`（连已有中央服务） |
| ADB / MCP / 未来协议 | 已有插件自动发现；新协议见 skill |
| 模拟（无硬件） | 无 flags，内置 `fake-001` 演示 |

## 7. 实战场景

### 7.1 给 U-Boot 发任意指令

```bash
elc exec --serial /dev/ttyUSB0 /dev/ttyUSB0 device.execute command="printenv"
```

### 7.2 批量刷写多台设备

```bash
elc batch device.flash partition=boot version=2.0.0 --devices dev-1,dev-2,dev-3 --concurrency 8
```

### 7.3 农场任务队列（异步、优先级）

```bash
elc serve --listen :8080 --serial /dev/ttyUSB0          # 中央
elc submit --grpc localhost:8080 device.flash partition=boot version=2.0.0 --priority 10
elc tasks --grpc localhost:8080                          # 轮询
elc pool  --grpc localhost:8080                          # 看设备池
```

## 8. MCP head（AI Agent 操作设备）

把设备能力暴露成标准 MCP tools，Claude / Cursor 等 Agent 能「看到并调用」设备能力：

```bash
elc mcp                          # 内置 fake 设备
elc mcp --serial /dev/ttyUSB0    # 真实串口设备
```

在 Claude / Cursor 的 MCP 配置里登记它（Claude Desktop `claude_desktop_config.json`）：

```json
{
  "mcpServers": {
    "hardware-channel": {
      "command": "elc",
      "args": ["mcp", "--serial", "/dev/ttyUSB0"]
    }
  }
}
```

之后 Agent 就能看到并调用 `device.info.get` / `device.flash` / `device.execute` 等工具，
每个工具的 `input_schema` = 能力 JSON Schema + 一个 `device` 目标参数。
**工具调用走完整 Core 治理**：deny-by-default、HIGH/CRITICAL 需审批、全链路审计。

## 9. 环境准备（WSL + CH340 串口示例）

1. Windows 管理员 PowerShell：`usbipd bind --busid 1-1` + `usbipd attach --wsl --busid 1-1`（把 CH340 透传进 WSL）。
2. WSL 里确认 `/dev/ttyUSB0` 出现。
3. 加 dialout 组：`sudo usermod -aG dialout $USER`，重启 WSL 生效；或临时 `sg dialout -c 'elc ...'`。
4. 验证：`elc exec --serial /dev/ttyUSB0 /dev/ttyUSB0 device.execute command="version"`。

## 10. 拓展新协议 / 新设备

遵循 skill **`add-channel-protocol`**（`.claude/skills/add-channel-protocol/SKILL.md`）：
评估连接形态 → 实现 `sdk.Plugin`（字节流用 `transport/console`）→ runtime 注册一行 → 合同测试。
架构门禁 `tests/arch` 保证你不会破坏 core/sdk 的协议无关性。
