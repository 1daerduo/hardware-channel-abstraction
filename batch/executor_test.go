package batch

import (
	"context"
	"testing"
	"time"

	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/fake"
	"example.com/embedded-loop-channel/runtime"
)

func threeDeviceRuntime(t *testing.T) (*runtime.Runtime, []*domain.Device) {
	t.Helper()
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("DEV-1", "board", "1.0", "usb:1-1.1"),
		fake.NewDevice("DEV-2", "board", "1.0", "usb:1-1.2"),
		fake.NewDevice("DEV-3", "board", "1.0", "usb:1-1.3"),
	))
	t.Cleanup(rt.Close)
	devices, err := rt.Client.Discover(ctx)
	if err != nil || len(devices) != 3 {
		t.Fatalf("discover: %d err=%v", len(devices), err)
	}
	return rt, devices
}

// TestBatchRunAll verifies a capability runs across all devices concurrently
// and aggregates the results.
func TestBatchRunAll(t *testing.T) {
	rt, devices := threeDeviceRuntime(t)
	rt.Client.Grant("batch", domain.CapabilityInfoGet)
	rt.Client.PreApprove("batch", domain.CapabilityInfoGet)

	sum, err := New(rt.Client).Run(context.Background(), Request{Capability: domain.CapabilityInfoGet})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if sum.Total != 3 || sum.Succeeded != 3 || sum.Failed != 0 {
		t.Fatalf("summary = %+v, want 3 succeeded", sum)
	}
	for i, r := range sum.Results {
		if r.State != domain.OperationSucceeded || r.Device.ID != devices[i].ID {
			t.Fatalf("result[%d] = %+v", i, r)
		}
	}
}

// TestBatchConcurrencyLimit verifies a bounded concurrency of 1 still runs
// every device correctly (serialized).
func TestBatchConcurrencyLimit(t *testing.T) {
	rt, _ := threeDeviceRuntime(t)
	rt.Client.Grant("batch", domain.CapabilityInfoGet)
	rt.Client.PreApprove("batch", domain.CapabilityInfoGet)

	sum, err := New(rt.Client).Run(context.Background(), Request{
		Capability: domain.CapabilityInfoGet, Concurrency: 1,
	})
	if err != nil || sum.Succeeded != 3 {
		t.Fatalf("summary = %+v err=%v, want 3 succeeded", sum, err)
	}
}

// TestBatchResourceScope verifies per-device failure is aggregated: a device
// the principal is NOT scoped to fails with DENY while others succeed.
func TestBatchResourceScope(t *testing.T) {
	rt, devices := threeDeviceRuntime(t)
	// Grant info.get on the first two devices only.
	rt.Client.GrantScope("batch", domain.CapabilityInfoGet, devices[0].ID)
	rt.Client.GrantScope("batch", domain.CapabilityInfoGet, devices[1].ID)
	rt.Client.PreApprove("batch", domain.CapabilityInfoGet)

	sum, err := New(rt.Client).Run(context.Background(), Request{Capability: domain.CapabilityInfoGet})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if sum.Succeeded != 2 || sum.Failed != 1 {
		t.Fatalf("summary = %+v, want 2 succeeded 1 failed", sum)
	}
	// The failed one is devices[2] (not scoped).
	if sum.Results[2].State == domain.OperationSucceeded {
		t.Fatalf("devices[2] should have failed (out of scope), got %s", sum.Results[2].State)
	}
	if sum.Results[2].Error == nil || sum.Results[2].Error.Category != domain.CategoryAuthorization {
		t.Fatalf("devices[2] error should be AUTHORIZATION, got %+v", sum.Results[2].Error)
	}
	_ = time.Second
}
