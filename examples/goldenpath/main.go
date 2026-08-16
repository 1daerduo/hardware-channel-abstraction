// Command goldenpath runs the reference Golden Path (Design doc 13 §44) and
// the multi-protocol acceptance scenario (Design doc 24):
//
//	Fake Device (USB-ADB + UART) → two Plugins → Discovery → identity
//	correlation → Session → device.info.get → device.reboot → device.flash
//	(risk-gated) → device.log (multi-channel resolution + override) →
//	device.console → device.reset → deny-by-default → Events → Evidence.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/core/event"
	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/runtime"
)

func main() {
	ctx := context.Background()

	// 1. Assemble the runtime with one board reachable over USB-ADB and UART.
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2").
			WithSerialPort("ttyUSB0").
			WithMCPURL("mcp://ABC123:8080"),
	))
	client := rt.Client

	// 2. Authorize two principals.
	client.Grant("agent", domain.CapabilityInfoGet)
	client.Grant("agent", domain.CapabilityReboot)
	client.Grant("agent", domain.CapabilityFlash)
	client.Grant("agent", domain.CapabilityLog)
	client.Grant("agent", domain.CapabilityConsole)
	client.Grant("agent", domain.CapabilityReset)
	client.Grant("viewer", domain.CapabilityInfoGet)

	// 3. Discovery: scan the farm, correlate identity, open channels. One
	// board → two endpoints → two channels, correlated to one Device.
	devices, err := client.Discover(ctx)
	must(err)
	fmt.Printf("== Discovery: %d device(s) ==\n", len(devices))
	device := devices[0]
	for _, d := range devices {
		fmt.Printf("  device %s  serial=%s model=%s\n", d.ID, d.Serial, d.Model)
		caps, _ := client.ListCapabilities(d.ID)
		fmt.Printf("  capabilities (union): %v\n", caps)
	}
	channels := rt.Registry.ChannelsByDevice(device.ID)
	fmt.Printf("  channels: %d (one board, multiple endpoints)\n", len(channels))
	for _, ch := range channels {
		fmt.Printf("    - %s type=%s state=%s cost=%d\n", ch.ID, ch.ChannelType, ch.State, ch.Cost)
	}

	// 4. Create a session for the agent principal.
	sess, err := client.CreateSession("agent", device.ID, 10*time.Minute)
	must(err)
	fmt.Printf("\n== Session %s (principal=%s) ==\n", sess.ID, sess.Principal)

	// 5. device.info.get (LOW) → ALLOW. Provided by both ADB and MCP; the
	// resolver picks ADB (cost 10 < 20) unless we override to MCP.
	info, err := client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: device.ID, SessionID: sess.ID,
	})
	fmt.Printf("\n== device.info.get (auto) ==\n")
	printResult(client, info, err)

	infoMCP, err := client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: device.ID, SessionID: sess.ID,
		ChannelType: "mcp", // third protocol, channel override
	})
	fmt.Printf("\n== device.info.get (channel override = mcp) ==\n")
	printResult(client, infoMCP, err)

	// 6. device.reboot (MEDIUM, exclusive device resource) → ALLOW.
	reboot, err := client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityReboot, Target: device.ID, SessionID: sess.ID,
	})
	fmt.Printf("\n== device.reboot ==\n")
	printResult(client, reboot, err)

	// 7. device.flash (HIGH) → REQUIRE_APPROVAL the first time.
	flashReq := domain.OperationRequest{
		Capability: domain.CapabilityFlash, Target: device.ID, SessionID: sess.ID,
		Parameters: map[string]string{"partition": "boot", "image": "boot-2.0.0.img", "version": "2.0.0"},
	}
	flashDenied, err := client.Execute(ctx, flashReq)
	fmt.Printf("\n== device.flash (before approval) ==\n")
	printResult(client, flashDenied, err)
	client.PreApprove("agent", domain.CapabilityFlash)
	flash, err := client.Execute(ctx, flashReq)
	fmt.Printf("\n== device.flash (after approval) ==\n")
	printResult(client, flash, err)
	printArtifacts(client, flash)

	// 8. device.log is offered by BOTH the ADB and UART channels. The resolver
	// ranks deterministically (healthy → cheaper → ID), so UART (cost 5) wins
	// over ADB (cost 10). Evidence records which channel was actually used.
	logReq := domain.OperationRequest{Capability: domain.CapabilityLog, Target: device.ID, SessionID: sess.ID}
	logAuto, err := client.Execute(ctx, logReq)
	fmt.Printf("\n== device.log (resolver auto-select) ==\n")
	printResult(client, logAuto, err)

	logReq.ChannelType = "adb" // advanced channel override (Design doc 11 §15)
	logAdb, err := client.Execute(ctx, logReq)
	fmt.Printf("\n== device.log (channel override = adb) ==\n")
	printResult(client, logAdb, err)

	// 9. UART-only capabilities: console (LOW) and reset (HIGH, approval).
	console, err := client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityConsole, Target: device.ID, SessionID: sess.ID,
	})
	fmt.Printf("\n== device.console (UART-only) ==\n")
	printResult(client, console, err)

	client.PreApprove("agent", domain.CapabilityReset)
	reset, err := client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityReset, Target: device.ID, SessionID: sess.ID,
	})
	fmt.Printf("\n== device.reset (UART-only, approved) ==\n")
	printResult(client, reset, err)

	// 10. deny-by-default: "viewer" has no execute grant.
	viewerSess, _ := client.CreateSession("viewer", device.ID, time.Minute)
	execRes, err := client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityExecute, Target: device.ID, SessionID: viewerSess.ID,
		Parameters: map[string]string{"command": "echo hello"},
	})
	fmt.Printf("\n== device.execute as viewer (deny-by-default) ==\n")
	printResult(client, execRes, err)

	// 11. Dump the device's event stream.
	fmt.Printf("\n== Event stream (device %s) ==\n", device.ID)
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
	if len(r.EvidenceRefs) > 0 {
		fmt.Printf("  evidence:\n")
		for _, id := range r.EvidenceRefs {
			if ev, ok := client.GetEvidence(id); ok {
				fmt.Printf("    - %s = %s\n", ev.Name, ev.Value)
			}
		}
	}
}

func printArtifacts(client interface {
	GetArtifact(domain.ArtifactID) (*domain.Artifact, bool)
	VerifyArtifact(domain.ArtifactID) (bool, error)
}, r *domain.OperationResult) {
	if r == nil || len(r.ArtifactRefs) == 0 {
		return
	}
	fmt.Printf("  artifacts:\n")
	for _, id := range r.ArtifactRefs {
		a, ok := client.GetArtifact(id)
		if !ok {
			continue
		}
		ok2, _ := client.VerifyArtifact(id)
		fmt.Printf("    - %s type=%s size=%d checksum=%s... verify=%t\n",
			a.ID, a.Type, a.SizeBytes, a.Checksum[:16], ok2)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
