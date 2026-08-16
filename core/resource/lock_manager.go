package resource

import (
	"context"
	"fmt"
	"sync"
	"time"

	"example.com/embedded-loop-channel/domain"
)

// Manager centralizes lock decisions (Design doc 07 §8). It supports SHARED
// and EXCLUSIVE modes with a lease TTL; expired leases are pruned lazily.
type Manager struct {
	mu      sync.Mutex
	cond    *sync.Cond
	leases  map[domain.ResourceID][]*domain.Lease
	byLease map[domain.LeaseID]*domain.Lease
	now     func() time.Time
}

// NewManager builds a Lock Manager using wall-clock time.
func NewManager() *Manager {
	m := &Manager{
		leases:  map[domain.ResourceID][]*domain.Lease{},
		byLease: map[domain.LeaseID]*domain.Lease{},
		now:     time.Now,
	}
	m.cond = sync.NewCond(&m.mu)
	return m
}

// Acquire blocks until holder obtains mode on resource, or ctx is done. On
// success it returns a lease expiring after ttl.
func (m *Manager) Acquire(ctx context.Context, holder string, resourceID domain.ResourceID, mode domain.LockMode, ttl time.Duration) (*domain.Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()

	// A watcher wakes all waiters when the context is cancelled, so Acquire
	// observes cancellation promptly.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			m.cond.Broadcast()
		case <-stop:
		}
	}()

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if l, ok := m.tryAcquireLocked(holder, resourceID, mode, ttl); ok {
			return l, nil
		}
		m.cond.Wait()
	}
}

// TryAcquire attempts a non-blocking acquire.
func (m *Manager) TryAcquire(holder string, resourceID domain.ResourceID, mode domain.LockMode, ttl time.Duration) (*domain.Lease, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	return m.tryAcquireLocked(holder, resourceID, mode, ttl)
}

// AcquireAll acquires multiple resources in the given (already-ordered) order,
// rolling back on failure. It blocks like Acquire.
func (m *Manager) AcquireAll(ctx context.Context, holder string, resources []domain.ResourceID, mode domain.LockMode, ttl time.Duration) ([]*domain.Lease, error) {
	var acquired []*domain.Lease
	for _, rid := range resources {
		l, err := m.Acquire(ctx, holder, rid, mode, ttl)
		if err != nil {
			m.releaseAll(acquired)
			return nil, err
		}
		acquired = append(acquired, l)
	}
	return acquired, nil
}

// Release releases a lease by ID.
func (m *Manager) Release(id domain.LeaseID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.byLease[id]
	if !ok {
		return fmt.Errorf("resource: lease %s not found", id)
	}
	delete(m.byLease, id)
	m.removeLeaseLocked(l)
	m.cond.Broadcast()
	return nil
}

// ReleaseAll releases a batch of leases.
func (m *Manager) ReleaseAll(leases []*domain.Lease) {
	m.releaseAll(leases)
	m.cond.Broadcast()
}

// Renew extends a lease, rejecting expired ones (Design doc 07 §7).
func (m *Manager) Renew(id domain.LeaseID, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.byLease[id]
	if !ok {
		return fmt.Errorf("resource: lease %s not found", id)
	}
	if l.Expired(m.now()) {
		delete(m.byLease, id)
		m.removeLeaseLocked(l)
		return fmt.Errorf("resource: lease %s expired", id)
	}
	l.ExpiresAt = m.now().Add(ttl)
	return nil
}

func (m *Manager) releaseAll(leases []*domain.Lease) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, l := range leases {
		delete(m.byLease, l.ID)
		m.removeLeaseLocked(l)
	}
}

func (m *Manager) tryAcquireLocked(holder string, resourceID domain.ResourceID, mode domain.LockMode, ttl time.Duration) (*domain.Lease, bool) {
	for _, l := range m.leases[resourceID] {
		if conflict(l.Mode, mode) {
			return nil, false
		}
	}
	lease := &domain.Lease{
		ID:         domain.NewLeaseID(),
		ResourceID: resourceID,
		Holder:     holder,
		Mode:       mode,
		ExpiresAt:  m.now().Add(ttl),
	}
	m.leases[resourceID] = append(m.leases[resourceID], lease)
	m.byLease[lease.ID] = lease
	return lease, true
}

func (m *Manager) removeLeaseLocked(l *domain.Lease) {
	list := m.leases[l.ResourceID]
	for i, x := range list {
		if x.ID == l.ID {
			m.leases[l.ResourceID] = append(list[:i], list[i+1:]...)
			return
		}
	}
}

func (m *Manager) pruneLocked() {
	now := m.now()
	for rid, list := range m.leases {
		kept := list[:0]
		for _, l := range list {
			if l.Expired(now) {
				delete(m.byLease, l.ID)
				continue
			}
			kept = append(kept, l)
		}
		m.leases[rid] = kept
	}
}

// conflict reports whether two lock modes conflict.
// EXCLUSIVE conflicts with everything; SHARED conflicts only with EXCLUSIVE.
func conflict(a, b domain.LockMode) bool {
	return a == domain.LockExclusive || b == domain.LockExclusive
}
