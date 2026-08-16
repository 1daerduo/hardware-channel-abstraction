package unit

import (
	"context"
	"testing"

	"example.com/embedded-loop-channel/core/event"
	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/fake"
	"example.com/embedded-loop-channel/runtime"
)

// TestHotplugRemoveAndReadd verifies the hotplug flow (Design doc 05 §15):
// remove → endpoint unavailable + channel degraded; re-add → channel READY.
func TestHotplugRemoveAndReadd(t *testing.T) {
	ctx := context.Background()
	dev := fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2")
	rt := runtime.Bootstrap(runtime.WithDevices(dev))
	c := rt.Client

	devices, err := c.Discover(ctx)
	if err != nil || len(devices) != 1 {
		t.Fatalf("discover failed: %v", err)
	}
	chs := rt.Registry.ChannelsByDevice(devices[0].ID)
	if len(chs) != 1 || chs[0].State != domain.ChannelReady {
		t.Fatalf("expected 1 READY channel, got %d state=%s", len(chs), chs[0].State)
	}

	// Removal: the device goes offline, so the scanner no longer sees it.
	dev.SetOnline(false)
	_, removed, err := c.Refresh(ctx)
	if err != nil || len(removed) != 1 {
		t.Fatalf("expected 1 removed device, got %d err=%v", len(removed), err)
	}
	if chs[0].State != domain.ChannelDegraded {
		t.Fatalf("channel should be DEGRADED after removal, got %s", chs[0].State)
	}

	// Re-add: the device comes back online.
	dev.SetOnline(true)
	added, _, err := c.Refresh(ctx)
	if err != nil || len(added) != 1 {
		t.Fatalf("expected 1 added device, got %d err=%v", len(added), err)
	}
	if chs[0].State != domain.ChannelReady {
		t.Fatalf("channel should be READY after re-add, got %s", chs[0].State)
	}

	// A DeviceOffline event must have been emitted during removal.
	var sawOffline bool
	for _, e := range c.Events(event.Filter{DeviceID: devices[0].ID}) {
		if e.Type == domain.EventDeviceOffline {
			sawOffline = true
		}
	}
	if !sawOffline {
		t.Fatalf("expected a DeviceOffline event")
	}
}
