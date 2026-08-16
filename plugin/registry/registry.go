// Package registry manages Plugin registration, manifest validation and
// capability lookup (Design doc 12 §28). Runtime code looks plugins up here
// rather than hard-coding imports.
package registry

import (
	"fmt"
	"sync"

	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/plugin/sdk"
)

// State is the plugin lifecycle managed by Core.
type State string

const (
	StateRegistered State = "REGISTERED"
	StateLoaded     State = "LOADED"
	StateReady      State = "READY"
	StateFailed     State = "FAILED"
)

type entry struct {
	plugin   sdk.Plugin
	manifest sdk.Manifest
	state    State
}

// Registry is a thread-safe plugin registry.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]*entry
}

// New builds an empty Registry.
func New() *Registry {
	return &Registry{plugins: map[string]*entry{}}
}

// Register validates and registers a plugin. It is idempotent per plugin ID:
// re-registering the same ID replaces the previous instance.
func (r *Registry) Register(p sdk.Plugin) error {
	m := p.Info()
	if err := m.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.plugins[m.ID]; exists {
		return fmt.Errorf("registry: plugin %q already registered", m.ID)
	}
	r.plugins[m.ID] = &entry{plugin: p, manifest: m, state: StateRegistered}
	return nil
}

// Load marks a plugin LOADED after dependency/trust checks (stubbed here).
func (r *Registry) Load(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.plugins[id]
	if !ok {
		return fmt.Errorf("registry: plugin %q not registered", id)
	}
	e.state = StateLoaded
	return nil
}

// Ready marks a plugin READY after a successful health check.
func (r *Registry) Ready(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.plugins[id]; ok {
		e.state = StateReady
	}
}

// Get returns a plugin by ID.
func (r *Registry) Get(id string) (sdk.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.plugins[id]
	if !ok {
		return nil, fmt.Errorf("registry: plugin %q not found", id)
	}
	return e.plugin, nil
}

// List returns the manifests of all registered plugins, in registration
// order.
func (r *Registry) List() []sdk.Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]sdk.Manifest, 0, len(r.plugins))
	for _, e := range r.plugins {
		out = append(out, e.manifest)
	}
	return out
}

// All returns every registered plugin instance, in registration order.
func (r *Registry) All() []sdk.Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]sdk.Plugin, 0, len(r.plugins))
	for _, e := range r.plugins {
		out = append(out, e.plugin)
	}
	return out
}

// FindByCapability returns the plugins that advertise the given capability.
func (r *Registry) FindByCapability(cap domain.CapabilityName) []sdk.Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []sdk.Plugin
	for _, e := range r.plugins {
		for _, c := range e.manifest.Capabilities {
			if c == cap {
				out = append(out, e.plugin)
				break
			}
		}
	}
	return out
}
