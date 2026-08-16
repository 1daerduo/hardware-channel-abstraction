// Command realserial drives a real serial-attached embedded device (e.g. an
// i.MX6ULL in U-Boot) through the UART Channel Plugin, proving the Channel
// Abstraction works against real hardware, not just the fake simulator.
//
// Run (after attaching the CH340 to WSL and joining the dialout group):
//
//	go run ./examples/realserial -path /dev/ttyUSB0 -baud 115200
package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/core/event"
	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/runtime"
)

func main() {
	path := flag.String("path", "/dev/ttyUSB0", "serial port path")
	baud := flag.Int("baud", 115200, "baud rate")
	flag.Parse()

	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithRealSerial(*path, *baud))
	defer rt.Close()

	client := rt.Client

	// device.execute is HIGH risk → grant + pre-approve.
	client.Grant("agent", domain.CapabilityExecute)
	client.Grant("agent", domain.CapabilityConsole)
	client.Grant("agent", domain.CapabilityLog)
	client.PreApprove("agent", domain.CapabilityExecute)

	devices, err := client.Discover(ctx)
	if err != nil || len(devices) == 0 {
		fmt.Printf("discovery failed: err=%v devices=%d\n", err, len(devices))
		return
	}
	device := devices[0]
	fmt.Printf("== Discovery: %d device(s) ==\n", len(devices))
	for _, d := range devices {
		fmt.Printf("  device %s  serial=%s model=%s\n", d.ID, d.Serial, d.Model)
	}

	sess, err := client.CreateSession("agent", device.ID, 10*time.Minute)
	if err != nil {
		fmt.Printf("session failed: %v\n", err)
		return
	}

	// 1. Read the console buffer.
	console, err := client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityConsole, Target: device.ID, SessionID: sess.ID,
	})
	fmt.Printf("\n== device.console ==\n")
	printResult(client, console, err)

	// 2. Run U-Boot commands via device.execute.
	for _, cmd := range []string{"version", "printenv bootargs", "mmc list"} {
		res, err := client.Execute(ctx, domain.OperationRequest{
			Capability: domain.CapabilityExecute, Target: device.ID, SessionID: sess.ID,
			Parameters: map[string]string{"command": cmd},
		})
		fmt.Printf("\n== device.execute %q ==\n", cmd)
		printResult(client, res, err)
	}

	// 3. Streaming console: open a live device.log stream, trigger output, and
	// read it back as sequenced lines.
	stream, err := client.OpenStream(ctx, device.ID, domain.CapabilityLog)
	if err != nil {
		fmt.Printf("open stream failed: %v\n", err)
		return
	}
	defer stream.Close("demo done")
	fmt.Printf("\n== device.log stream (id=%s) ==\n", stream.ID())

	// Trigger some output on the console.
	client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityExecute, Target: device.ID, SessionID: sess.ID,
		Parameters: map[string]string{"command": "printenv bootargs"},
	})

	// Drain a few streamed lines.
	readCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	for i := 0; i < 6; i++ {
		chunk, err := stream.Read(readCtx)
		if err != nil {
			fmt.Printf("  (read: %v)\n", err)
			break
		}
		if chunk.Closed {
			fmt.Printf("  [closed: %s]\n", chunk.CloseReason)
			break
		}
		fmt.Printf("  #%d %q\n", chunk.Sequence, string(chunk.Data))
	}
	fmt.Printf("  cursor=%d\n", stream.Cursor())

	// 3. Show the event stream for this device.
	fmt.Printf("\n== Event stream ==\n")
	for _, e := range client.Events(event.Filter{DeviceID: device.ID}) {
		fmt.Printf("  #%02d %-22s %s\n", e.Sequence, e.Type, e.Subject)
	}
}

func printResult(client interface {
	GetEvidence(domain.EvidenceID) (*domain.Evidence, bool)
}, r *domain.OperationResult, err error) {
	if err != nil {
		fmt.Printf("  result: %s\n", r.State)
		fmt.Printf("  error:  %s\n", err)
		return
	}
	fmt.Printf("  result: %s\n", r.State)
	fmt.Printf("  output: %q\n", r.Output)
}
