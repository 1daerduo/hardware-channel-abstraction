// Command tcpdevice verifies the "portable extension" promise end-to-end: a
// simulated TCP console device is discovered and driven through the SAME
// Unified API as ADB/UART/MCP — the consumer never knows it's TCP.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/runtime"
)

func main() {
	ctx := context.Background()

	// 1. 启动一个模拟的 TCP 控制台设备。
	dev, err := fake.NewConsoleServer("127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer dev.Close()
	fmt.Printf("模拟 TCP 设备监听: %s\n", dev.Addr())

	// 2. 通过统一 API 接入（唯一声明点：一个地址）。
	rt := runtime.Bootstrap(runtime.WithTCPDevice(dev.Addr()))
	defer rt.Close()
	c := rt.Client
	c.Grant("agent", domain.CapabilityInfoGet)
	c.Grant("agent", domain.CapabilityReboot)
	c.Grant("agent", domain.CapabilityExecute)
	c.PreApprove("agent", domain.CapabilityExecute)

	// 3. 发现（消费者只看到一台设备，不知道它是 TCP）。
	devices, err := c.Discover(ctx)
	if err != nil || len(devices) == 0 {
		fmt.Printf("discovery failed: err=%v devices=%d\n", err, len(devices))
		return
	}
	d := devices[0]
	fmt.Printf("== Discovery: 1 台设备 ==\n  %s serial=%s model=%s\n", d.ID, d.Serial, d.Model)
	caps, _ := c.ListCapabilities(d.ID)
	fmt.Printf("  capabilities: %v\n", caps)

	sess, _ := c.CreateSession("agent", d.ID, time.Minute)

	// 4. 执行统一能力。
	type op struct {
		cap    domain.CapabilityName
		params map[string]string
	}
	for _, o := range []op{
		{domain.CapabilityInfoGet, nil},
		{domain.CapabilityReboot, nil},
		{domain.CapabilityExecute, map[string]string{"command": "echo hello-over-tcp"}},
	} {
		res, err := c.Execute(ctx, domain.OperationRequest{
			Capability: o.cap, Target: d.ID, SessionID: sess.ID, Parameters: o.params,
		})
		if err != nil {
			fmt.Printf("\n== %s ==\n  error: %v\n", o.cap, err)
			continue
		}
		fmt.Printf("\n== %s ==\n  result: %s\n  output: %q\n", o.cap, res.State, res.Output)
	}
}
