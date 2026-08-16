package discovery

import (
	"testing"

	"github.com/1daerduo/hardware-channel-abstraction/core/event"
	"github.com/1daerduo/hardware-channel-abstraction/core/registry"
	"github.com/1daerduo/hardware-channel-abstraction/domain"
	pluginregistry "github.com/1daerduo/hardware-channel-abstraction/plugin/registry"
)

// TestCorrelateIdentityConflictQuarantines verifies a strong identity conflict
// (same serial, different hardware ID) is quarantined, never force-merged, and
// emits a diagnostic event (Design doc 05 §10).
func TestCorrelateIdentityConflictQuarantines(t *testing.T) {
	reg := registry.New()
	bus := event.New()
	s := New(pluginregistry.New(), reg, bus)

	d1 := s.correlate(map[string]string{"serial": "ABC123", "hardware_id": "HW-X", "model": "eval"})
	if d1.State != domain.DeviceStateOnline {
		t.Fatalf("first device should be ONLINE, got %s", d1.State)
	}

	d2 := s.correlate(map[string]string{"serial": "ABC123", "hardware_id": "HW-Y", "model": "eval"})
	if d2.State != domain.DeviceStateQuarantined {
		t.Fatalf("conflicting device should be QUARANTINED, got %s", d2.State)
	}
	if d2.ID == d1.ID {
		t.Fatalf("conflict must NOT merge into the existing device")
	}

	found := false
	for range bus.Events(event.Filter{Type: domain.EventIdentityConflict}) {
		found = true
	}
	if !found {
		t.Fatalf("expected an IdentityConflict diagnostic event")
	}
}

// TestCorrelateMatchingHardwareReuses verifies a matching hardware ID reuses
// the existing device (no quarantine).
func TestCorrelateMatchingHardwareReuses(t *testing.T) {
	reg := registry.New()
	bus := event.New()
	s := New(pluginregistry.New(), reg, bus)

	d1 := s.correlate(map[string]string{"serial": "ABC123", "hardware_id": "HW-X", "model": "eval"})
	d2 := s.correlate(map[string]string{"serial": "ABC123", "hardware_id": "HW-X", "model": "eval"})
	if d1.ID != d2.ID {
		t.Fatalf("matching hardware ID must reuse the same device")
	}
}
