package unit

import (
	"context"
	"testing"
	"time"

	"example.com/embedded-loop-channel/core/resource"
	"example.com/embedded-loop-channel/domain"
)

func TestLockExclusiveConflict(t *testing.T) {
	m := resource.NewManager()
	rid := domain.ResourceID("res-1")

	l1, ok := m.TryAcquire("a", rid, domain.LockExclusive, time.Minute)
	if !ok || l1 == nil {
		t.Fatalf("first exclusive acquire should succeed")
	}
	if _, ok := m.TryAcquire("b", rid, domain.LockExclusive, time.Minute); ok {
		t.Fatalf("second exclusive acquire should conflict")
	}

	// Blocking acquire respects context cancellation while conflicted.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := m.Acquire(ctx, "b", rid, domain.LockExclusive, time.Minute); err == nil {
		t.Fatalf("blocked acquire should observe context cancellation")
	}

	if err := m.Release(l1.ID); err != nil {
		t.Fatalf("release failed: %v", err)
	}
	if _, ok := m.TryAcquire("b", rid, domain.LockExclusive, time.Minute); !ok {
		t.Fatalf("acquire after release should succeed")
	}
}

func TestLockSharedAllowsShared(t *testing.T) {
	m := resource.NewManager()
	rid := domain.ResourceID("res-1")

	if _, ok := m.TryAcquire("a", rid, domain.LockShared, time.Minute); !ok {
		t.Fatalf("first shared acquire should succeed")
	}
	if _, ok := m.TryAcquire("b", rid, domain.LockShared, time.Minute); !ok {
		t.Fatalf("shared+shared should coexist")
	}
	if _, ok := m.TryAcquire("c", rid, domain.LockExclusive, time.Minute); ok {
		t.Fatalf("exclusive must conflict with existing shared")
	}
}

func TestLockAcquireAllRollsBack(t *testing.T) {
	m := resource.NewManager()
	r1 := domain.ResourceID("res-1")
	r2 := domain.ResourceID("res-2")

	// Hold r2 exclusively.
	if _, ok := m.TryAcquire("holder", r2, domain.LockExclusive, time.Minute); !ok {
		t.Fatalf("setup acquire failed")
	}

	// AcquireAll(r1, r2) must fail on r2 and release r1.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := m.AcquireAll(ctx, "x", []domain.ResourceID{r1, r2}, domain.LockExclusive, time.Minute)
	if err == nil {
		t.Fatalf("AcquireAll should fail on r2")
	}
	// r1 must be free again.
	if _, ok := m.TryAcquire("y", r1, domain.LockExclusive, time.Minute); !ok {
		t.Fatalf("r1 should have been rolled back")
	}
}
