// Command elc is the command-line head of the device capability layer: a
// human operator drives devices through the SAME sdk.ConnectivityAPI that a
// Loop or an Agent uses. The backend (fake / serial / TCP / gRPC) is chosen by
// flags; the CLI code itself is transport-agnostic.
//
// Usage:
//
//	elc devices                        # 发现并列出设备
//	elc caps <device>                  # 列出设备能力
//	elc exec <device> <capability> [k=v ...]   # 执行一个能力
//	elc stream <device> <capability>   # 流式控制台（仅进程内后端）
//
// Backend flags (任意子命令均可带):
//
//	--grpc <addr>         连接远程 gRPC 服务
//	--serial <path>       真实串口（--baud 默认 115200）
//	--tcp <addr>          TCP 设备
//	（无 flags 时默认使用一个内置 fake 设备，便于演示）
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/fake"
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
	case "devices", "discover":
		runDevices(os.Args[2:])
	case "caps", "capabilities":
		runCaps(os.Args[2:])
	case "exec":
		runExec(os.Args[2:])
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
  elc devices                         发现并列出设备
  elc caps <device>                   列出设备能力
  elc exec <device> <capability> [k=v ...]   执行一个能力
  elc stream <device> <capability>    流式控制台（仅进程内后端）

后端 flags（任意子命令均可带）:
  --grpc <addr>      连接远程 gRPC 服务
  --serial <path>    真实串口（--baud 默认 115200）
  --tcp <addr>       TCP 设备
  （无 flags 时默认内置 fake 设备）

示例:
  elc devices
  elc exec fake-001 device.info.get
  elc exec fake-001 device.flash partition=boot image=boot.img version=2.0.0
  elc --tcp 127.0.0.1:58732 exec <设备> device.execute command="echo hi"
`)
}

// backend selects the ConnectivityAPI implementation from flags.
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

// connect builds a ConnectivityAPI and returns it plus a cleanup func.
func (b *backend) connect() (sdk.ConnectivityAPI, func(), error) {
	if b.grpcAddr != "" {
		conn, err := grpc.NewClient(b.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, nil, err
		}
		return grpctransport.NewClient(conn), func() { _ = conn.Close() }, nil
	}

	var opts []runtime.Option
	if b.serial != "" {
		opts = append(opts, runtime.WithRealSerial(b.serial, b.baud))
	}
	if b.tcpAddr != "" {
		opts = append(opts, runtime.WithTCPDevice(b.tcpAddr))
	}
	// 默认内置一台 fake 设备，便于无 flags 演示。
	opts = append(opts, runtime.WithDevices(fake.NewDevice("fake-001", "demo-board", "1.0", "usb:1-1.1")))
	rt := runtime.Bootstrap(opts...)
	grant(rt.Client)
	return rt.Client, rt.Close, nil
}

// grant authorizes the CLI principal on the in-process backend.
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

// findDevice matches a device by ID or serial (exact or prefix). It triggers a
// discovery first if the registry is empty.
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

func runDevices(args []string) {
	fs := flag.NewFlagSet("devices", flag.ExitOnError)
	var b backend
	b.addFlags(fs)
	_ = fs.Parse(args)

	api, cleanup, err := b.connect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接失败: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	devices, err := api.Discover(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "发现失败: %v\n", err)
		os.Exit(1)
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
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "用法: elc caps <device>")
		os.Exit(2)
	}

	api, cleanup, err := b.connect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接失败: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	d, err := findDevice(api, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	caps, err := api.ListCapabilities(d.ID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, c := range caps {
		fmt.Println(c)
	}
}

func runExec(args []string) {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	var b backend
	b.addFlags(fs)
	_ = fs.Parse(args)
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "用法: elc exec <device> <capability> [k=v ...]")
		os.Exit(2)
	}

	api, cleanup, err := b.connect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接失败: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	d, err := findDevice(api, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	params := map[string]string{}
	for _, a := range fs.Args()[2:] {
		if i := strings.Index(a, "="); i >= 0 {
			params[a[:i]] = a[i+1:]
		}
	}

	sess, err := api.CreateSession(principal, d.ID, time.Minute)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建会话失败: %v\n", err)
		os.Exit(1)
	}

	res, err := api.Execute(context.Background(), domain.OperationRequest{
		Capability: domain.CapabilityName(fs.Arg(1)),
		Target:     d.ID,
		SessionID:  sess.ID,
		Parameters: params,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "执行失败: %s / %v\n", res.State, err)
		os.Exit(1)
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

func runStream(args []string) {
	fs := flag.NewFlagSet("stream", flag.ExitOnError)
	var b backend
	b.addFlags(fs)
	_ = fs.Parse(args)
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "用法: elc stream <device> <capability>")
		os.Exit(2)
	}
	if b.grpcAddr != "" {
		fmt.Fprintln(os.Stderr, "流式仅支持进程内后端（不使用 --grpc）")
		os.Exit(1)
	}

	// 流式需要 *sdk.Client（OpenStream 不在 ConnectivityAPI 上）。
	var opts []runtime.Option
	if b.serial != "" {
		opts = append(opts, runtime.WithRealSerial(b.serial, b.baud))
	}
	if b.tcpAddr != "" {
		opts = append(opts, runtime.WithTCPDevice(b.tcpAddr))
	}
	opts = append(opts, runtime.WithDevices(fake.NewDevice("fake-001", "demo-board", "1.0", "usb:1-1.1")))
	rt := runtime.Bootstrap(opts...)
	defer rt.Close()
	c := rt.Client

	d, err := findDevice(c, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	stream, err := c.OpenStream(context.Background(), d.ID, domain.CapabilityName(fs.Arg(1)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开流失败: %v\n", err)
		os.Exit(1)
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
