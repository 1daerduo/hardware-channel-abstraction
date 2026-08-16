// Package contract holds the reusable Plugin Contract Test (Design doc 17).
// Every plugin, regardless of protocol, must pass AssertPluginContract.
package contract

import (
	"context"
	"errors"
	"testing"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/adb"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/jtag"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/mcp"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/uart"
)

// asDomainErr asserts err is a normalized *domain.Error and returns it.
func asDomainErr(t *testing.T, err error) *domain.Error {
	t.Helper()
	var de *domain.Error
	if !errors.As(err, &de) {
		t.Fatalf("expected *domain.Error, got %T: %v", err, err)
	}
	return de
}

// Case parameterizes the contract test per protocol.
type Case struct {
	EndpointType     domain.EndpointType
	Locator          string
	ChannelType      string
	SampleCapability domain.CapabilityName
	// ForeignEndpointType is the endpoint type a plugin must reject. Defaults
	// to EndpointJTAG (fine for most plugins; the JTAG plugin itself uses a
	// different foreign type).
	ForeignEndpointType domain.EndpointType
}

// AssertPluginContract runs the plugin SPI contract against p backed by farm.
// A plugin that fails any assertion does not satisfy the Channel SPI.
func AssertPluginContract(t *testing.T, p sdk.Plugin, farm *fake.Farm, c Case) {
	t.Helper()
	ctx := context.Background()

	// 1. Manifest contract: stable ID, api_version, capabilities, trust.
	m := p.Info()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest invalid: %v", err)
	}

	// 2. Probe contract: correct endpoint matches; wrong endpoint does not.
	good := domain.Endpoint{ID: domain.NewEndpointID(), Type: c.EndpointType, Locator: c.Locator}
	res, err := p.Probe(ctx, good)
	if err != nil || !res.Match || res.Confidence <= 0 {
		t.Fatalf("probe should match known endpoint: res=%+v err=%v", res, err)
	}
	unknown := domain.Endpoint{ID: domain.NewEndpointID(), Type: c.EndpointType, Locator: "nowhere"}
	if res, _ := p.Probe(ctx, unknown); res.Match {
		t.Fatalf("probe should reject unknown locator")
	}
	foreign := c.ForeignEndpointType
	if foreign == "" {
		foreign = domain.EndpointJTAG
	}
	wrongType := domain.Endpoint{ID: domain.NewEndpointID(), Type: foreign, Locator: c.Locator}
	if res, _ := p.Probe(ctx, wrongType); res.Match {
		t.Fatalf("probe should reject a foreign endpoint type")
	}

	// 3. Capability contract: at least one capability, valid metadata.
	caps := p.Capabilities(nil)
	if len(caps) == 0 {
		t.Fatalf("plugin must advertise at least one capability")
	}
	for _, cap := range caps {
		if cap.Name == "" || cap.Version == "" || cap.RiskLevel == "" {
			t.Fatalf("capability %+v has empty required field", cap)
		}
	}

	// 4. Channel lifecycle: open → READY+healthy; close → CLOSED.
	ch := domain.NewChannel(m.ID, c.ChannelType, good.ID, domain.DeviceID("dev-x"))
	ch.Locator = good.Locator
	if err := p.Open(ctx, ch, ""); err != nil {
		t.Fatalf("open failed: %v", err)
	}
	if ch.State != domain.ChannelReady || !ch.Healthy {
		t.Fatalf("channel should be READY+healthy after open: %+v", ch)
	}
	if err := p.Health(ctx, ch); err != nil {
		t.Fatalf("health failed on healthy channel: %v", err)
	}

	// 5. Invoke contract: typed result + evidence, normalized errors.
	inv, err := p.Invoke(ctx, ch, sdk.InvokeRequest{
		Capability: c.SampleCapability,
		Target:     domain.DeviceID("dev-x"),
		Parameters: map[string]string{},
	})
	if err != nil {
		t.Fatalf("invoke %s failed: %v", c.SampleCapability, err)
	}
	if inv.Output == "" || len(inv.Evidence) == 0 {
		t.Fatalf("invoke should return output and evidence")
	}

	// Unsupported capability → normalized error (CategoryValidation).
	if _, err := p.Invoke(ctx, ch, sdk.InvokeRequest{Capability: domain.CapabilityName("nope")}); err == nil {
		t.Fatalf("invoke of unsupported capability should error")
	} else if de := asDomainErr(t, err); de.Category != domain.CategoryValidation {
		t.Fatalf("unsupported capability should map to VALIDATION, got %s", de.Category)
	}

	// 6. Error mapping: offline device → DEVICE_STATE, not raw text.
	dev := farm.ByLocator(good.Locator)
	dev.SetOnline(false)
	if err := p.Health(ctx, ch); err == nil {
		t.Fatalf("health on offline device should error")
	} else if de := asDomainErr(t, err); de.Category != domain.CategoryDeviceState {
		t.Fatalf("offline device should map to DEVICE_STATE, got %s", de.Category)
	}
	if _, err := p.Invoke(ctx, ch, sdk.InvokeRequest{Capability: c.SampleCapability}); err == nil {
		t.Fatalf("invoke on offline device should error")
	} else if de := asDomainErr(t, err); de.Category != domain.CategoryDeviceState {
		t.Fatalf("invoke on offline device should map to DEVICE_STATE, got %s", de.Category)
	}

	// 7. Recovery contract: after the device returns, recover re-binds.
	dev.SetOnline(true)
	if err := p.Recover(ctx, ch, "test"); err != nil {
		t.Fatalf("recover failed: %v", err)
	}
	if ch.State != domain.ChannelReady || !ch.Healthy {
		t.Fatalf("channel should be READY after recover: %+v", ch)
	}

	if err := p.Close(ctx, ch); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if ch.State != domain.ChannelClosed {
		t.Fatalf("channel should be CLOSED after close: %+v", ch)
	}
}

func TestADBPluginContract(t *testing.T) {
	farm := fake.NewFarm()
	farm.Add(fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2"))
	AssertPluginContract(t, adb.New(farm), farm, Case{
		EndpointType:     domain.EndpointUSBADB,
		Locator:          "usb:1-1.2",
		ChannelType:      "adb",
		SampleCapability: domain.CapabilityInfoGet,
	})
}

func TestUARTPluginContract(t *testing.T) {
	farm := fake.NewFarm()
	farm.Add(fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2").
		WithSerialPort("ttyUSB0"))
	AssertPluginContract(t, uart.New(farm), farm, Case{
		EndpointType:     domain.EndpointUART,
		Locator:          "ttyUSB0",
		ChannelType:      "uart",
		SampleCapability: domain.CapabilityConsole,
	})
}

func TestMCPPluginContract(t *testing.T) {
	farm := fake.NewFarm()
	farm.Add(fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2").
		WithMCPURL("mcp://ABC123:8080"))
	AssertPluginContract(t, mcp.New(farm), farm, Case{
		EndpointType:     domain.EndpointMCP,
		Locator:          "mcp://ABC123:8080",
		ChannelType:      "mcp",
		SampleCapability: domain.CapabilityInfoGet,
	})
}

func TestJTAGPluginContract(t *testing.T) {
	farm := fake.NewFarm()
	farm.Add(fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2").
		WithJTAGLocator("debug:1-1.2"))
	AssertPluginContract(t, jtag.NewWithResolver(farm), farm, Case{
		EndpointType:        domain.EndpointJTAG,
		Locator:             "debug:1-1.2",
		ChannelType:         "jtag",
		SampleCapability:    domain.CapabilityDebugHalt,
		ForeignEndpointType: domain.EndpointUART,
	})
}
