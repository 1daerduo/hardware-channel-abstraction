// Package registry holds the runtime registries for Devices, Endpoints and
// Channels (Design doc 05 §11). It is the state the Discovery/Resolver write
// and the Operation/Session layers read.
package registry

import (
	"sort"
	"sync"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// Registry is a thread-safe store of devices, endpoints and channels.
type Registry struct {
	mu        sync.RWMutex
	devices   map[domain.DeviceID]*domain.Device
	bySerial  map[string]domain.DeviceID
	endpoints map[domain.EndpointID]*domain.Endpoint
	channels  map[domain.ChannelID]*domain.Channel
}

// New builds an empty Registry.
func New() *Registry {
	return &Registry{
		devices:   map[domain.DeviceID]*domain.Device{},
		bySerial:  map[string]domain.DeviceID{},
		endpoints: map[domain.EndpointID]*domain.Endpoint{},
		channels:  map[domain.ChannelID]*domain.Channel{},
	}
}

// UpsertDevice inserts or updates a device and its serial index.
func (r *Registry) UpsertDevice(d *domain.Device) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.devices[d.ID] = d
	if d.Serial != "" {
		r.bySerial[d.Serial] = d.ID
	}
}

// GetDevice returns a device by ID.
func (r *Registry) GetDevice(id domain.DeviceID) (*domain.Device, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.devices[id]
	return d, ok
}

// DeviceBySerial resolves a device by its strong identity signal.
func (r *Registry) DeviceBySerial(serial string) (*domain.Device, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.bySerial[serial]
	if !ok {
		return nil, false
	}
	d, ok := r.devices[id]
	return d, ok
}

// ListDevices returns all devices in stable serial order.
func (r *Registry) ListDevices() []*domain.Device {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*domain.Device, 0, len(r.devices))
	for _, d := range r.devices {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Serial < out[j].Serial })
	return out
}

// AddEndpoint records an endpoint and links it to its device.
func (r *Registry) AddEndpoint(e *domain.Endpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endpoints[e.ID] = e
	if d, ok := r.devices[e.DeviceID]; ok {
		d.AddEndpoint(e.ID)
	}
}

// GetEndpoint returns an endpoint by ID.
func (r *Registry) GetEndpoint(id domain.EndpointID) (*domain.Endpoint, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.endpoints[id]
	return e, ok
}

// ListEndpoints returns all registered endpoints.
func (r *Registry) ListEndpoints() []*domain.Endpoint {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*domain.Endpoint, 0, len(r.endpoints))
	for _, e := range r.endpoints {
		out = append(out, e)
	}
	return out
}

// EndpointByLocator returns an endpoint by its transport locator.
func (r *Registry) EndpointByLocator(locator string) (*domain.Endpoint, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.endpoints {
		if e.Locator == locator {
			return e, true
		}
	}
	return nil, false
}

// ChannelsByEndpoint returns all channels bound to an endpoint.
func (r *Registry) ChannelsByEndpoint(endpointID domain.EndpointID) []*domain.Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*domain.Channel
	for _, c := range r.channels {
		if c.EndpointID == endpointID {
			out = append(out, c)
		}
	}
	return out
}

// AddChannel records a channel.
func (r *Registry) AddChannel(c *domain.Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[c.ID] = c
}

// GetChannel returns a channel by ID.
func (r *Registry) GetChannel(id domain.ChannelID) (*domain.Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.channels[id]
	return c, ok
}

// ChannelsByDevice returns all channels bound to a device.
func (r *Registry) ChannelsByDevice(deviceID domain.DeviceID) []*domain.Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*domain.Channel
	for _, c := range r.channels {
		if c.DeviceID == deviceID {
			out = append(out, c)
		}
	}
	return out
}

// ChannelsByDeviceCapability returns READY channels of a device that
// advertise the given capability.
func (r *Registry) ChannelsByDeviceCapability(deviceID domain.DeviceID, cap domain.CapabilityName) []*domain.Channel {
	var out []*domain.Channel
	for _, c := range r.ChannelsByDevice(deviceID) {
		if c.State == domain.ChannelReady && c.Supports(cap) {
			out = append(out, c)
		}
	}
	return out
}
