# Embedded Loop Channel Abstraction — Go MVP

Go + Protobuf 实现的「嵌入式 Loop 通道抽象层」MVP 骨架，忠实落盘
`Embedded_Loop_Channel_Abstraction_Final_Baseline_HTML` 中的 01~18 设计文档。

## 一句话

上层（Embedded Loop / Agent / CLI / SDK）通过**统一 API** 访问异构嵌入式设备，
把 ADB / UART / JTAG / TCP / MCP 等协议差异隔离在 **Channel Plugin SPI** 之后。

核心抽象链：**Endpoint → Channel → Capability → Operation**；
Session / Lock / Policy / Health / Event / Artifact / Recovery 提供工程治理。

## 分层

```
Consumer (Loop / Agent / CLI)          ← 只面向 ConnectivityAPI（抽象）
        │
  Unified API (sdk.ConnectivityAPI)
        │   ├─ 进程内:   sdk.Client
        │   └─ 远程:     transport/grpc.Client      ← 便携拓展（传输可插拔）
        ↓
  Connectivity Core (core/*)
        ↓
  Channel Plugin SPI (plugin/sdk)
        │   ├─ plugin/adb   (USB/ADB)
        │   ├─ plugin/uart  (串口控制台)
        │   └─ plugin/mcp   (远程服务)
        ↓
  Transport (fake 模拟 / transport/serial 真实串口)
        ↓
  Device / Endpoint (domain)
```

## 目录结构（对应设计文档 13）

| 目录 | 职责 | 对应文档 |
|---|---|---|
| `api/proto` + `api/gen` | Canonical Schema/IDL（Protobuf + gRPC service） | 14 |
| `api/convert` | domain ↔ proto 转换 | 14 |
| `domain/` | Device/Endpoint/Channel/Capability/Operation/Session/Resource/Error/Event/Artifact | 02, 06, 07, 08, 09 |
| `plugin/sdk` | Plugin SPI 契约 + Manifest + ConsoleDevice/Stream/Canceller | 04, 12, 16 |
| `plugin/registry` | 插件注册/加载/校验 | 12 |
| `plugin/adb` | 参考 ADB-like Plugin（USB/ADB） | 12 |
| `plugin/uart` | 参考 UART/串口 Plugin（串口控制台 + 流式） | 12, 24 |
| `plugin/mcp` | 参考 MCP 远程服务 Plugin（第三种协议） | 12, 24 |
| `transport/serial` | 真实串口 ConsoleDevice（U-Boot 命令行/回显/单读泵流式） | 12 |
| `transport/grpc` | gRPC 传输（ConnectivityService 薄适配 + 远程客户端） | 03, 14 |
| `fake/` | 内存设备模拟器（Fake Device，USB-ADB + UART + MCP 三 Endpoint） | 17, 18 |
| `core/discovery` | 发现 + 身份关联 + 热插拔 Refresh/Watch + 冲突隔离 | 05 |
| `core/resolver` | 能力 → Channel 选路（确定性排序 + override） | 05, 12 |
| `core/session` | 会话生命周期 | 07 |
| `core/resource` | 资源注册 + 锁/租约（SHARED/EXCLUSIVE） | 07 |
| `core/operation` | Operation 状态机 + 执行编排 + 异步/取消 | 06, 13 |
| `core/event` | 事件总线 + 至少一次投递 + Cursor | 08 |
| `core/artifact` | Artifact 存储（upload/finalize/checksum/verify） | 08, 15 |
| `core/security` | 认证(AuthN) + deny-by-default + 资源范围 + 密钥 + 审批 + 审计 | 10 |
| `core/recovery` | 错误分类 + L0~L6 恢复阶梯 + State Reconciliation | 09 |
| `sdk/` | 统一 API（ConnectivityAPI 接口 + 客户端） | 03, 15 |
| `runtime/` | 装配根（Bootstrap） | 13 §38 |
| `examples/goldenpath` | Golden Path 可运行示例（三协议模拟） | 13 §44 |
| `examples/realserial` | 真实串口驱动示例（真机 U-Boot + 流式） | 12 |
| `tests/contract` | 可复用的 Plugin Contract Test（三协议） | 17 |
| `tests/unit` | 领域/选路/锁/幂等/转换/异步/安全/gRPC 测试 | 17 |
| `tests/arch` | 架构门禁（依赖方向 + 协议特判静态检查） | 17 §15 |

## 关键设计不变量（已实现）

- **UNKNOWN 是一等状态**：`OperationUnknown` 是终态且不是成功。
- **deny-by-default**：未授权能力一律 `DENY`；HIGH/CRITICAL 需审批。
- **Retry 是 Policy**：业务重试由 Core 控制，恢复受 `maxAttempts` 预算约束，
  Plugin 不自行无限重试。
- **依赖方向**：`domain ← core ← sdk/runtime`，`plugin → plugin/sdk`；
  Core 无 `if protocol == ADB` 特判，Loop 不依赖 Plugin。
- **错误结构化**：统一 `code/category/retryable/recoverable/severity`，
  底层 raw 文本只进 `details`。

## 已实现能力总览（对照设计文档，除 Loop Runtime 消费方外全部落地）

| 能力 | 落地 | 文档 |
|---|---|---|
| 统一抽象链 | Endpoint → Channel → Capability → Operation + 全领域模型 | 02 |
| 统一 API | `sdk.ConnectivityAPI` 接口（进程内 + gRPC 远程两种实现） | 03 |
| Plugin SPI | `plugin/sdk`：Probe/Open/Invoke/Health/Observe/Recover/Cancel/Stream | 04, 12, 16 |
| 三协议接入 | ADB / UART / MCP，Core 零改动 + Contract Test | 12, 24 |
| 发现 + 热插拔 | Discovery/Refresh/Watch + 身份关联 + 冲突隔离(Quarantine) | 05 |
| 能力选路 | Capability → Channel 确定性排序 + override | 05, 12 |
| 可靠执行 | Operation 状态机 + 幂等 + 异步(Start/Wait) + 取消(Cancel) | 06 |
| 并发治理 | Session / Lock(SHARED/EXCLUSIVE) / Lease / 死锁防护 | 07 |
| 事件与产物 | EventBus(序列/游标) + Artifact(checksum/verify) + Evidence + 流式 | 08 |
| 恢复 | L0~L6 阶梯 + Observe-first + State Reconciliation + Budget/Backoff | 09 |
| 安全 | AuthN + deny-by-default + 资源范围 + 密钥(SecretRef/Redact) + 审批 + 审计 | 10 |
| 传输网络化 | gRPC ConnectivityService（薄适配，与进程内共享同一接口） | 03, 14 |
| 真机验证 | i.MX6ULL U-Boot 通过统一 API 执行 + 流式控制台 | 12 |
| 测试门禁 | Contract / Unit / Architecture(依赖方向+协议特判) / Mutation | 17 |

## 如何拓展一种新协议（便携拓展）

架构的红线：**协议差异只进 Plugin；core/sdk 不出现协议名、不依赖具体插件。**
新增一种连接方式（例如 JTAG / TCP / 新 Vendor）只需：

1. 新建 `plugin/<proto>/`，实现 `plugin/sdk.Plugin`（含 `Manifest` / `Probe` /
   `Open/Close` / `Invoke` / `Health` / `Observe` / `Recover` / 可选 `Cancel`/`Stream`）。
2. 定义该协议的 `Capability` 映射（统一能力 → 协议动作，如 `device.reset` → JTAG reset）。
3. 错误归一化（raw 错误 → `domain.Error` 分类），Secret 走 `SecretRef`。
4. 在 `runtime/bootstrap.go` 注册该 Plugin（装配根唯一改点）。
5. 写一条 `AssertPluginContract(t, plugin, ...)` 合同测试。
6. `go test ./...` 全绿 → 架构门禁会校验你没有改 core/sdk。

新增一种「传输」（真实设备）同理：实现 `plugin/sdk.ConsoleDevice`（或 `StreamProvider`），
在 runtime 的 resolver/scanner 里登记即可，Plugin 逻辑不变（`plugin/uart` 同时驱动
`fake.Device` 与真实串口就是证明）。

## 构建与测试

```bash
export PATH=$HOME/.local/go/bin:$HOME/.local/protoc/bin:$PATH

go build ./...        # 编译全部包
go test ./...         # 单元 + 契约 + 转换测试
go run ./examples/goldenpath   # 运行 Golden Path
```

## 命令行（消费者之头）

```bash
go run ./cmd/elc devices                                    # 发现并列出设备
go run ./cmd/elc exec fake-001 device.info.get              # 执行能力
go run ./cmd/elc exec fake-001 device.flash partition=boot image=boot.img version=2.0.0
go run ./cmd/elc --tcp 127.0.0.1:58732 exec <设备> device.execute command="echo hi"
go run ./cmd/elc --grpc <addr> devices                      # 远程后端
```

重新生成 protobuf（消息 + gRPC service）：

```bash
protoc -I api/proto \
  --go_out=. --go_opt=module=example.com/embedded-loop-channel \
  --go-grpc_out=. --go-grpc_opt=module=example.com/embedded-loop-channel \
  api/proto/channel/v1/channel.proto api/proto/channel/v1/service.proto
```

## Golden Path 输出要点

```
== Discovery: 1 device(s) ==          # 一个板子 → USB-ADB + UART + MCP 三个 Endpoint
  channels: 3 (adb cost=10, uart cost=5, mcp cost=20)  # serial 身份关联合并为同一 Device
== device.info.get (auto) == SUCCEEDED                 # ADB(10) < MCP(20)
== device.info.get (override=mcp) == SUCCEEDED, mcp.tool=get_device_info
== device.reboot == SUCCEEDED         # MEDIUM, 独占 device 资源
== device.flash (before approval) == VALIDATION_FAILED / PERMISSION_DENIED
== device.flash (after approval) == SUCCEEDED   # HIGH, evidence: flash.version=2.0.0
  artifacts: flash-report checksum=... verify=true     # Artifact 存储 + 校验
== device.log (auto) == SUCCEEDED, log.source=uart      # 多通道自动选路(便宜者胜)
== device.log (override=adb) == SUCCEEDED, log.source=adb
== device.console / device.reset (UART-only) == SUCCEEDED
== device.execute as viewer == PERMISSION_DENIED        # deny-by-default
== Event stream == ChannelReady / Audit / OperationStarted / OperationSucceeded / ...
```

验证了文档 24 的验收标准：**三种性质不同的协议（ADB/UART/MCP）只各新增一个
Plugin + Contract Test，Core / Loop 零改动**；**同一 Capability 由多个 Channel
提供并自动选路 + override**。设备掉线时走 **Observe-first → L2 重连 → L5 设备
恢复 → 对账**，未知结果不会被伪造为成功或失败。

## 真实串口（真机验证）

UART Plugin 是传输无关的（依赖 `sdk.ConsoleDevice` 接口），同一个 Plugin 既驱动
`fake.Device` 也能驱动真实串口。`transport/serial` 实现了真实串口（发命令 + 读
U-Boot 提示符回显）。

```bash
# 前置：usbipd 把 CH340 attach 进 WSL，且用户加入 dialout 组
go run ./examples/realserial -path /dev/ttyUSB0 -baud 115200
```

实测（i.MX6ULL EVK @ U-Boot 2016.03）：

```
== Discovery: 1 device(s) ==  serial=/dev/ttyUSB0 model=embedded
== device.execute "version" ==        U-Boot 2016.03-g6a0dab4-dirty ...
== device.execute "printenv bootargs" == bootargs=console=ttymxc0,115200 ...
== device.execute "mmc list" ==       FSL_SDHC: 0 (SD) / FSL_SDHC: 1
```

即：真机 U-Boot 的 `device.execute` 通过统一 API 执行，Discovery → Session →
Policy → Resolver → UART Plugin → 真实串口 → U-Boot 全链路打通。

流式控制台（文档 08 §10 / 12 §22 的 Stream Contract）：`sdk.Client.OpenStream` 打开
设备 `device.log`/`device.console` 的实时流，按行输出，带 **stream_id / sequence /
cursor / close_reason**。串口 Console 用单读泵（唯一 reader），`Execute` 与 Stream
共享同一字节流：

```
== device.log stream (id=serial-/dev/ttyUSB0-0) ==
  #1 "printenv bootargs"                              # 回显
  #2 "bootargs=console=ttymxc0,115200 ..."            # 输出（换行已归一化）
  cursor=2
```

## 路线图（对照设计文档 18）

- **已完成（本仓库）**：核心抽象链、统一 API（进程内 + gRPC）、Plugin SPI、
  Discovery/热插拔/冲突隔离、Resolver、Session/Lock/Lease、Operation（同步/异步/取消）、
  Event/Artifact/Evidence/流式、Recovery L0~L6 + 对账、安全（认证/资源范围/密钥/审批/审计）、
  三种参考 Plugin（ADB/UART/MCP）、真机验证、Contract/Unit/Architecture/Mutation 测试。
- **下一步（Beta）**：真实 ADB/MCP 客户端替换 `fake.Farm`、Artifact Retention/过期、
  完整 Approval 工作流、真实设备 E2E 全矩阵。
- **Production**：多租户、Plugin 沙箱、动态升级、审计不可变存储、HA。

## 验收标准（设计文档 24）

新增一种协议 = 只新增一个 Plugin + 通过 Contract Test，不修改 Core；
新增一个 Capability = 不改 Loop DSL；设备掉线不产生错误成功；未知结果可 Reconcile；
高风险动作可审计。
