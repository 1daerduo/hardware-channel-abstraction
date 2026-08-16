package farm

import (
	"context"
	"testing"
	"time"

	"example.com/embedded-loop-channel/batch"
	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/fake"
	"example.com/embedded-loop-channel/runtime"
)

func farmRuntime(t *testing.T) (*runtime.Runtime, []*domain.Device) {
	t.Helper()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("DEV-1", "board", "1.0", "usb:1-1.1"),
		fake.NewDevice("DEV-2", "board", "1.0", "usb:1-1.2"),
		fake.NewDevice("DEV-3", "board", "1.0", "usb:1-1.3"),
	))
	t.Cleanup(rt.Close)
	devices, err := rt.Client.Discover(context.Background())
	if err != nil || len(devices) != 3 {
		t.Fatalf("discover: %d err=%v", len(devices), err)
	}
	rt.Client.Grant("farm", domain.CapabilityInfoGet)
	rt.Client.Grant("farm", domain.CapabilityReboot)
	rt.Client.PreApprove("farm", domain.CapabilityInfoGet)
	rt.Client.PreApprove("farm", domain.CapabilityReboot)
	return rt, devices
}

func waitFor(t *testing.T, s *Scheduler, id string, want TaskState) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		task, err := s.Status(id)
		if err == nil && task.State == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task %s did not reach %s", id, want)
}

// TestSchedulerRunsTasksInPriorityOrder verifies queued tasks run to success
// and the pool records per-device last state.
func TestSchedulerRunsTasks(t *testing.T) {
	rt, _ := farmRuntime(t)
	s := New(rt.Client, 2)
	s.Start()
	defer s.Stop()

	id, err := s.Submit(batch.Request{Capability: domain.CapabilityInfoGet, Principal: "farm"}, 0)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, s, id, TaskSucceeded)

	task, _ := s.Status(id)
	if task.Summary == nil || task.Summary.Succeeded != 3 {
		t.Fatalf("summary = %+v, want 3 succeeded", task.Summary)
	}

	pool := s.PoolSnapshot()
	if len(pool) != 3 {
		t.Fatalf("pool size = %d, want 3", len(pool))
	}
	for _, e := range pool {
		if e.LastState != domain.OperationSucceeded || e.Busy {
			t.Fatalf("pool entry = %+v", e)
		}
	}
}

// TestSchedulerPriorityOrder verifies higher-priority tasks run before lower.
func TestSchedulerPriorityOrder(t *testing.T) {
	rt, _ := farmRuntime(t)
	s := New(rt.Client, 1) // one worker → strict serialization
	s.Start()
	defer s.Stop()

	low, _ := s.Submit(batch.Request{Capability: domain.CapabilityInfoGet, Principal: "farm"}, 0)
	high, _ := s.Submit(batch.Request{Capability: domain.CapabilityReboot, Principal: "farm"}, 10)
	waitFor(t, s, high, TaskSucceeded)
	waitFor(t, s, low, TaskSucceeded)

	// The high-priority task should have started before the low one.
	ht, _ := s.Status(high)
	lt, _ := s.Status(low)
	if !ht.StartedAt.Before(lt.StartedAt) {
		t.Fatalf("priority order violated: high started %v, low started %v", ht.StartedAt, lt.StartedAt)
	}
}

// TestSchedulerCancel verifies a PENDING task can be cancelled.
func TestSchedulerCancel(t *testing.T) {
	rt, _ := farmRuntime(t)
	s := New(rt.Client, 1)
	s.Start()
	defer s.Stop()

	id, _ := s.Submit(batch.Request{Capability: domain.CapabilityInfoGet, Principal: "farm"}, 0)
	if err := s.Cancel(id); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	task, _ := s.Status(id)
	if task.State != TaskCancelled {
		t.Fatalf("state = %s, want CANCELLED", task.State)
	}
}
