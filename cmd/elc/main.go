// Command elc is the command-line head of the device capability layer: a
// human operator drives devices through the SAME sdk.ConnectivityAPI that a
// Loop or an Agent uses. The backend (fake / serial / TCP / gRPC) is chosen by
// flags; the CLI code itself is transport-agnostic.
//
// Usage:
//
//	elc serve [--listen :8080] [--serial ...] [--tcp ...]   # 起 gRPC 服务（农场中央）
//	elc devices [--json]                                    # 发现并列出设备
//	elc caps <device> [--json]                              # 列出能力（含描述+schema）
//	elc exec <device> <capability> [k=v ...] [--json]       # 执行能力
//	elc stream <device> <capability>                        # 流式控制台（进程内）
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"example.com/embedded-loop-channel/batch"
	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/fake"
	"example.com/embedded-loop-channel/farm"
	"example.com/embedded-loop-channel/runtime"
	"example.com/embedded-loop-channel/sdk"
	grpctransport "example.com/embedded-loop-channel/transport/grpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const principal = "cli"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "devices", "discover":
		runDevices(os.Args[2:])
	case "caps", "capabilities":
		runCaps(os.Args[2:])
	case "exec":
		runExec(os.Args[2:])
	case "batch":
		runBatch(os.Args[2:])
	case "queue":
		runQueue(os.Args[2:])
	case "stream":
		runStream(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`elc — 设备能力层命令行

用法:
  elc serve [--listen :8080] [--serial ...] [--tcp ...]   起 gRPC 服务
  elc devices [--json]                                    发现并列出设备
  elc caps <device> [--json]                              列出能力（含描述+schema）
  elc exec <device> <capability> [k=v ...] [--json]       执行能力
  elc batch <capability> [k=v ...] [--devices a,b] [--concurrency N] [--json]   批量执行
  elc queue <capability> [k=v ...] [--priority N]         任务队列（提交+等待+设备池）
  elc stream <device> <capability>                        流式控制台（进程内）

后端 flags:
  --grpc <addr>       连接远程 gRPC 服务（serve 起的）
  --serial <path>     真实串口（--baud 默认 115200）
  --tcp <addr>        TCP 设备
  （无 flags 时默认内置 fake 设备）

示例:
  elc devices
  elc caps fake-001
  elc exec fake-001 device.info.get
  elc exec fake-001 device.flash partition=boot image=boot.img version=2.0.0
  elc serve --listen :8080
  elc devices --grpc localhost:8080        # 连接 serve 起的远程服务
`)
}

type backend struct {
	grpcAddr string
	serial   string
	baud     int
	tcpAddr  string
}

func (b *backend) addFlags(fs *flag.FlagSet) {
	fs.StringVar(&b.grpcAddr, "grpc", "", "gRPC 服务地址")
	fs.StringVar(&b.serial, "serial", "", "真实串口路径")
	fs.IntVar(&b.baud, "baud", 115200, "串口波特率")
	fs.StringVar(&b.tcpAddr, "tcp", "", "TCP 设备地址")
}

func (b *backend) runtimeOpts() []runtime.Option {
	var opts []runtime.Option
	if b.serial != "" {
		opts = append(opts, runtime.WithRealSerial(b.serial, b.baud))
	}
	if b.tcpAddr != "" {
		opts = append(opts, runtime.WithTCPDevice(b.tcpAddr))
	}
	opts = append(opts, runtime.WithDevices(fake.NewDevice("fake-001", "demo-board", "1.0", "usb:1-1.1")))
	return opts
}

// bootstrap builds an in-process runtime and authorizes the CLI principal.
func (b *backend) bootstrap() (*runtime.Runtime, error) {
	rt := runtime.Bootstrap(b.runtimeOpts()...)
	grant(rt.Client)
	return rt, nil
}

// connect builds a ConnectivityAPI (remote gRPC client or in-process runtime).
func (b *backend) connect() (sdk.ConnectivityAPI, func(), error) {
	if b.grpcAddr != "" {
		conn, err := grpc.NewClient(b.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, nil, err
		}
		return grpctransport.NewClient(conn), func() { _ = conn.Close() }, nil
	}
	rt, err := b.bootstrap()
	if err != nil {
		return nil, nil, err
	}
	return rt.Client, rt.Close, nil
}

func grant(c *sdk.Client) {
	for _, cap := range []domain.CapabilityName{
		domain.CapabilityInfoGet, domain.CapabilityReboot, domain.CapabilityFlash,
		domain.CapabilityExecute, domain.CapabilityConsole, domain.CapabilityLog,
		domain.CapabilityReset, domain.CapabilityFileRead,
	} {
		c.Grant(principal, cap)
		c.PreApprove(principal, cap)
	}
}

// valueFlags are flags that consume a following argument as their value.
var valueFlags = map[string]bool{"grpc": true, "serial": true, "baud": true, "tcp": true, "listen": true}

func flagName(a string) string {
	a = strings.TrimLeft(a, "-")
	if i := strings.Index(a, "="); i >= 0 {
		a = a[:i]
	}
	return a
}

// reorderArgs moves flags before positional args, so `elc caps fake-001 --json`
// works the same as `elc caps --json fake-001` (Go's flag stops at the first
// positional). It keeps value-taking flags paired with their values.
func reorderArgs(args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 1 && a[0] == '-' {
			flags = append(flags, a)
			if valueFlags[flagName(a)] && i+1 < len(args) && !strings.Contains(a, "=") {
				i++
				flags = append(flags, args[i])
			}
		} else {
			pos = append(pos, a)
		}
	}
	return append(flags, pos...)
}

func findDevice(api sdk.ConnectivityAPI, ref string) (*domain.Device, error) {
	devices := api.ListDevices()
	if len(devices) == 0 {
		devices, _ = api.Discover(context.Background())
	}
	for _, d := range devices {
		if string(d.ID) == ref || d.Serial == ref ||
			strings.HasPrefix(string(d.ID), ref) || strings.HasPrefix(d.Serial, ref) {
			return d, nil
		}
	}
	return nil, fmt.Errorf("未找到设备 %q（先 elc devices）", ref)
}

// ---------------------------------------------------------------------------
// serve
// ---------------------------------------------------------------------------

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var b backend
	b.addFlags(fs)
	listen := fs.String("listen", ":8080", "gRPC 监听地址")
	_ = fs.Parse(reorderArgs(args))

	rt, err := b.bootstrap()
	if err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
	defer rt.Close()

	srv := grpctransport.NewServer(rt.Client)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("elc serve 监听 %s（Ctrl+C 退出）\n", *listen)
	if err := srv.Serve(ctx, *listen); err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// devices / caps / exec / stream
// ---------------------------------------------------------------------------

func runDevices(args []string) {
	fs := flag.NewFlagSet("devices", flag.ExitOnError)
	var b backend
	b.addFlags(fs)
	jsonMode := fs.Bool("json", false, "JSON 输出")
	_ = fs.Parse(reorderArgs(args))

	api, cleanup, err := b.connect()
	if err != nil {
		fatalf("连接失败: %v", err)
	}
	defer cleanup()

	devices, err := api.Discover(context.Background())
	if err != nil {
		fatalf("发现失败: %v", err)
	}

	if *jsonMode {
		type out struct {
			ID           string   `json:"id"`
			Serial       string   `json:"serial"`
			Model        string   `json:"model"`
			State        string   `json:"state"`
			Capabilities []string `json:"capabilities"`
		}
		var rows []out
		for _, d := range devices {
			caps, _ := api.ListCapabilities(d.ID)
			names := make([]string, 0, len(caps))
			for _, c := range caps {
				names = append(names, string(c))
			}
			rows = append(rows, out{string(d.ID), d.Serial, d.Model, string(d.State), names})
		}
		printJSON(rows)
		return
	}

	for _, d := range devices {
		caps, _ := api.ListCapabilities(d.ID)
		fmt.Printf("%s  serial=%s  model=%s  state=%s\n", d.ID, d.Serial, d.Model, d.State)
		fmt.Printf("  能力: %v\n", caps)
	}
}

func runCaps(args []string) {
	fs := flag.NewFlagSet("caps", flag.ExitOnError)
	var b backend
	b.addFlags(fs)
	jsonMode := fs.Bool("json", false, "JSON 输出（工具定义）")
	_ = fs.Parse(reorderArgs(args))
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "用法: elc caps <device> [--json]")
		os.Exit(2)
	}

	api, cleanup, err := b.connect()
	if err != nil {
		fatalf("连接失败: %v", err)
	}
	defer cleanup()

	d, err := findDevice(api, fs.Arg(0))
	if err != nil {
		fatalf("%v", err)
	}

	caps, err := api.DescribeCapabilities(d.ID)
	if err != nil {
		fatalf("%v", err)
	}

	if *jsonMode {
		tools := make([]map[string]any, 0, len(caps))
		for i := range caps {
			tools = append(tools, caps[i].ToolDefinition())
		}
		printJSON(tools)
		return
	}

	for _, c := range caps {
		fmt.Printf("%s — %s  [%s, idempotent=%t]\n", c.Name, c.Description, c.RiskLevel, c.Idempotent)
		if c.InputSchema != nil {
			fmt.Printf("  input: %s\n", c.InputSchema.JSON())
		}
	}
}

func runExec(args []string) {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	var b backend
	b.addFlags(fs)
	jsonMode := fs.Bool("json", false, "JSON 输出")
	_ = fs.Parse(reorderArgs(args))
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "用法: elc exec <device> <capability> [k=v ...] [--json]")
		os.Exit(2)
	}

	api, cleanup, err := b.connect()
	if err != nil {
		fatalf("连接失败: %v", err)
	}
	defer cleanup()

	d, err := findDevice(api, fs.Arg(0))
	if err != nil {
		fatalf("%v", err)
	}

	params := map[string]string{}
	for _, a := range fs.Args()[2:] {
		if i := strings.Index(a, "="); i >= 0 {
			params[a[:i]] = a[i+1:]
		}
	}

	sess, err := api.CreateSession(principal, d.ID, time.Minute)
	if err != nil {
		fatalf("创建会话失败: %v", err)
	}

	res, err := api.Execute(context.Background(), domain.OperationRequest{
		Capability: domain.CapabilityName(fs.Arg(1)),
		Target:     d.ID,
		SessionID:  sess.ID,
		Parameters: params,
	})

	if *jsonMode {
		out := map[string]any{}
		if res != nil {
			out["state"] = res.State
			out["output"] = res.Output
			out["evidence"] = res.EvidenceRefs
			out["artifacts"] = res.ArtifactRefs
		}
		if err != nil {
			out["error"] = err.Error()
		}
		printJSON(out)
		if err != nil {
			os.Exit(1)
		}
		return
	}

	if err != nil {
		fatalf("执行失败: %s / %v", res.State, err)
	}
	fmt.Printf("状态: %s\n", res.State)
	if res.Output != "" {
		fmt.Printf("输出:\n%s\n", res.Output)
	}
	if len(res.EvidenceRefs) > 0 {
		fmt.Println("证据:", res.EvidenceRefs)
	}
	if len(res.ArtifactRefs) > 0 {
		fmt.Println("产物:", res.ArtifactRefs)
	}
}

func runBatch(args []string) {
	fs := flag.NewFlagSet("batch", flag.ExitOnError)
	var b backend
	b.addFlags(fs)
	jsonMode := fs.Bool("json", false, "JSON 输出")
	devicesFlag := fs.String("devices", "", "逗号分隔的设备 serial/ID（空=全部）")
	conc := fs.Int("concurrency", 0, "并发度（0=设备数）")
	_ = fs.Parse(reorderArgs(args))
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "用法: elc batch <capability> [k=v ...] [--devices a,b] [--concurrency N] [--json]")
		os.Exit(2)
	}

	api, cleanup, err := b.connect()
	if err != nil {
		fatalf("连接失败: %v", err)
	}
	defer cleanup()

	params := map[string]string{}
	for _, a := range fs.Args()[1:] {
		if i := strings.Index(a, "="); i >= 0 {
			params[a[:i]] = a[i+1:]
		}
	}

	var deviceIDs []domain.DeviceID
	if *devicesFlag != "" {
		for _, s := range strings.Split(*devicesFlag, ",") {
			if s = strings.TrimSpace(s); s != "" {
				deviceIDs = append(deviceIDs, domain.DeviceID(s))
			}
		}
	}

	sum, err := batch.New(api).Run(context.Background(), batch.Request{
		Capability:  domain.CapabilityName(fs.Arg(0)),
		Parameters:  params,
		Devices:     deviceIDs,
		Principal:   principal,
		Concurrency: *conc,
	})
	if err != nil {
		fatalf("批量执行失败: %v", err)
	}

	if *jsonMode {
		type row struct {
			Device string `json:"device"`
			State  string `json:"state"`
			Error  string `json:"error,omitempty"`
		}
		rows := make([]row, 0, len(sum.Results))
		for _, r := range sum.Results {
			e := ""
			if r.Error != nil {
				e = r.Error.Error()
			}
			rows = append(rows, row{r.Device.Serial, string(r.State), e})
		}
		printJSON(map[string]any{
			"capability": fs.Arg(0), "total": sum.Total, "succeeded": sum.Succeeded, "failed": sum.Failed,
			"results": rows,
		})
		return
	}

	fmt.Printf("批量 %s: %d 台设备, 成功 %d, 失败 %d\n", fs.Arg(0), sum.Total, sum.Succeeded, sum.Failed)
	for _, r := range sum.Results {
		status := "OK"
		if r.State != domain.OperationSucceeded {
			status = "FAIL " + string(r.State)
			if r.Error != nil {
				status += " (" + r.Error.Error() + ")"
			}
		}
		fmt.Printf("  %-12s %s\n", r.Device.Serial, status)
	}
}

func runQueue(args []string) {
	fs := flag.NewFlagSet("queue", flag.ExitOnError)
	var b backend
	b.addFlags(fs)
	priority := fs.Int("priority", 0, "任务优先级（越高越先）")
	_ = fs.Parse(reorderArgs(args))
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "用法: elc queue <capability> [k=v ...] [--priority N]")
		os.Exit(2)
	}
	if b.grpcAddr != "" {
		fmt.Fprintln(os.Stderr, "queue 当前仅支持进程内（队列常驻在 serve 进程，gRPC 队列 RPC 待实现）")
		os.Exit(1)
	}

	rt, err := b.bootstrap()
	if err != nil {
		fatalf("启动失败: %v", err)
	}
	defer rt.Close()

	params := map[string]string{}
	for _, a := range fs.Args()[1:] {
		if i := strings.Index(a, "="); i >= 0 {
			params[a[:i]] = a[i+1:]
		}
	}

	s := farm.New(rt.Client, 1)
	s.Start()
	defer s.Stop()

	id, err := s.Submit(batch.Request{
		Capability: domain.CapabilityName(fs.Arg(0)),
		Parameters: params,
		Principal:  principal,
	}, *priority)
	if err != nil {
		fatalf("提交失败: %v", err)
	}
	fmt.Printf("已提交任务 %s\n", id)

	for {
		task, _ := s.Status(id)
		if task.State == farm.TaskSucceeded || task.State == farm.TaskFailed || task.State == farm.TaskCancelled {
			fmt.Printf("任务 %s: %s\n", id, task.State)
			if task.Summary != nil {
				fmt.Printf("  total=%d succeeded=%d failed=%d\n", task.Summary.Total, task.Summary.Succeeded, task.Summary.Failed)
			}
			if task.Err != "" {
				fmt.Printf("  error: %s\n", task.Err)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	fmt.Println("设备池状态:")
	for _, e := range s.PoolSnapshot() {
		busy := "idle"
		if e.Busy {
			busy = "busy"
		}
		last := string(e.LastState)
		if last == "" {
			last = "-"
		}
		fmt.Printf("  %-12s %-4s last=%s\n", e.Device.Serial, busy, last)
	}
}

func runStream(args []string) {
	fs := flag.NewFlagSet("stream", flag.ExitOnError)
	var b backend
	b.addFlags(fs)
	_ = fs.Parse(reorderArgs(args))
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "用法: elc stream <device> <capability>")
		os.Exit(2)
	}
	if b.grpcAddr != "" {
		fmt.Fprintln(os.Stderr, "流式仅支持进程内后端（不使用 --grpc）")
		os.Exit(1)
	}

	rt, err := b.bootstrap()
	if err != nil {
		fatalf("启动失败: %v", err)
	}
	defer rt.Close()
	c := rt.Client

	d, err := findDevice(c, fs.Arg(0))
	if err != nil {
		fatalf("%v", err)
	}
	stream, err := c.OpenStream(context.Background(), d.ID, domain.CapabilityName(fs.Arg(1)))
	if err != nil {
		fatalf("打开流失败: %v", err)
	}
	defer stream.Close("cli done")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		chunk, err := stream.Read(ctx)
		if err != nil {
			break
		}
		if chunk.Closed {
			break
		}
		fmt.Printf("#%d %s\n", chunk.Sequence, chunk.Data)
	}
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
