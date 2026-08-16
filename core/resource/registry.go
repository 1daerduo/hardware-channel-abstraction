// Package resource implements the Resource Registry and Lock Manager
// (Design doc 07). Resources are governed objects; locks are ownership
// relationships; leases are their automatic expiry.
package resource

import (
	"sync"

	"example.com/embedded-loop-channel/domain"
)

// Registry maps (device, type) to a concrete Resource instance. It is
// idempotent: the first Ensure creates the resource, later calls return it.
type Registry struct {
	mu           sync.RWMutex
	byID         map[domain.ResourceID]*domain.Resource
	byDeviceType map[domain.DeviceID]map[string]domain.ResourceID
}

// NewRegistry builds an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		byID:         map[domain.ResourceID]*domain.Resource{},
		byDeviceType: map[domain.DeviceID]map[string]domain.ResourceID{},
	}
}

// Ensure returns the Resource for (device, type), creating it on first use.
func (r *Registry) Ensure(deviceID domain.DeviceID, typ string) *domain.Resource {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byDeviceType[deviceID] == nil {
		r.byDeviceType[deviceID] = map[string]domain.ResourceID{}
	}
	if id, ok := r.byDeviceType[deviceID][typ]; ok {
		return r.byID[id]
	}
	res := domain.NewResource(typ, deviceID)
	r.byID[res.ID] = res
	r.byDeviceType[deviceID][typ] = res.ID
	return res
}

// Get returns the Resource for (device, type), if registered.
func (r *Registry) Get(deviceID domain.DeviceID, typ string) (*domain.Resource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.byDeviceType[deviceID] == nil {
		return nil, false
	}
	id, ok := r.byDeviceType[deviceID][typ]
	if !ok {
		return nil, false
	}
	res, ok := r.byID[id]
	return res, ok
}

// GetByID returns a Resource by its ID.
func (r *Registry) GetByID(id domain.ResourceID) (*domain.Resource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res, ok := r.byID[id]
	return res, ok
}
