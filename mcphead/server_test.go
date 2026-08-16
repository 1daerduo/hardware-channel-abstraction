package mcphead

import (
	"context"
	"testing"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/runtime"
)

func TestToolSchemaAddsDeviceParam(t *testing.T) {
	cap := domain.Capability{
		Name:        domain.CapabilityFlash,
		Description: "刷写固件",
		InputSchema: domain.ObjectSchema(
			[]string{"partition"},
			map[string]domain.JSONSchema{"partition": domain.StringSchema("分区")},
		),
	}
	s := toolSchema(cap)
	props, _ := s["properties"].(map[string]any)
	if _, ok := props["device"]; !ok {
		t.Fatalf("missing device param: %v", props)
	}
	req, _ := s["required"].([]any)
	hasDevice, hasPartition := false, false
	for _, r := range req {
		if r == "device" {
			hasDevice = true
		}
		if r == "partition" {
			hasPartition = true
		}
	}
	if !hasDevice || !hasPartition {
		t.Fatalf("required = %v, want [partition device]", req)
	}
}

func TestHandleCallExecutes(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("DEV-1", "board", "1.0", "usb:1-1.1"),
	))
	defer rt.Close()
	rt.Client.Grant("mcp", domain.CapabilityInfoGet)
	rt.Client.PreApprove("mcp", domain.CapabilityInfoGet)

	devices, _ := rt.Client.Discover(ctx)
	if len(devices) != 1 {
		t.Fatalf("discover: %d", len(devices))
	}

	cap := domain.Capability{
		Name:        domain.CapabilityInfoGet,
		Description: "读取设备信息",
		InputSchema: domain.ObjectSchema(nil, nil),
		RiskLevel:   domain.RiskLow,
	}
	res, err := handleCall(ctx, rt.Client, cap, Options{Principal: "mcp"}, map[string]any{
		"device": devices[0].Serial,
	})
	if err != nil {
		t.Fatalf("handleCall: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("result = %+v, want success", res)
	}
}

func TestHandleCallBadDevice(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap()
	defer rt.Close()
	cap := domain.Capability{Name: domain.CapabilityInfoGet}
	res, err := handleCall(ctx, rt.Client, cap, Options{}, map[string]any{"device": "nope"})
	if err != nil {
		t.Fatalf("handleCall: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected error result, got %+v", res)
	}
}

func TestBuildRegistersTools(t *testing.T) {
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("DEV-1", "board", "1.0", "usb:1-1.1"),
	))
	defer rt.Close()
	s, err := Build(rt.Client, Options{Principal: "mcp", SessionTTL: time.Minute})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	tools := s.ListTools()
	// ADB fake device exposes 6 capabilities → 6 tools.
	if len(tools) != 6 {
		t.Fatalf("tools = %d, want 6", len(tools))
	}
	if _, ok := tools["device.flash"]; !ok {
		t.Fatalf("missing device.flash tool")
	}
}
