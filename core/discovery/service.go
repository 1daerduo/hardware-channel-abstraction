// Package discovery implements the Discovery pipeline (Design doc 05):
// Scan → Probe → Identity Correlation → Device/Endpoint registration →
// Channel creation (open + health) → Events, plus hotplug refresh and
// identity-conflict quarantine.
package discovery

import (
	"context"
	"sync"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/core/event"
	"github.com/1daerduo/hardware-channel-abstraction/core/registry"
	"github.com/1daerduo/hardware-channel-abstraction/domain"
	pluginregistry "github.com/1daerduo/hardware-channel-abstraction/plugin/registry"
)

// Scanner enumerates raw endpoints from one discovery source (USB, serial,
// network, a fake device farm...). It returns endpoints with an empty
// DeviceID; Discovery assigns identity via correlation.
type Scanner interface {
	Scan(ctx context.Context) ([]domain.Endpoint, error)
}

// Service runs discovery over one or more scanners.
type Service struct {
	plugins  *pluginregistry.Registry
	reg      *registry.Registry
	bus      *event.Bus
	scanners []Scanner

	mu       sync.Mutex
	lastSeen map[string]bool // locator -> observed in the last scan
}

// New builds a Discovery Service.
func New(plugins *pluginregistry.Registry, reg *registry.Registry, bus *event.Bus) *Service {
	return &Service{
		plugins:  plugins,
		reg:      reg,
		bus:      bus,
		lastSeen: map[string]bool{},
	}
}

// AddScanner registers a discovery source.
func (s *Service) AddScanner(sc Scanner) { s.scanners = append(s.scanners, sc) }

// scan collects endpoints from all scanners.
func (s *Service) scan(ctx context.Context) []domain.Endpoint {
	var all []domain.Endpoint
	for _, sc := range s.scanners {
		eps, err := sc.Scan(ctx)
		if err != nil {
			continue
		}
		all = append(all, eps...)
	}
	return all
}

// Discover scans all sources and registers/updates Devices and Channels. It
// returns the devices observed in this pass and records the seen locators.
func (s *Service) Discover(ctx context.Context) ([]*domain.Device, error) {
	endpoints := s.scan(ctx)
	locators := map[string]bool{}
	for _, ep := range endpoints {
		locators[ep.Locator] = true
	}
	s.mu.Lock()
	s.lastSeen = locators
	s.mu.Unlock()
	return s.discoverEndpoints(ctx, endpoints)
}

// Refresh detects hotplug add/remove: endpoints no longer observed are marked
// unavailable and their channels degraded; newly observed endpoints are
// discovered. It returns the added and removed devices.
func (s *Service) Refresh(ctx context.Context) (added, removed []*domain.Device, err error) {
	endpoints := s.scan(ctx)
	current := map[string]bool{}
	for _, ep := range endpoints {
		current[ep.Locator] = true
	}

	s.mu.Lock()
	prev := s.lastSeen
	for loc := range prev {
		if current[loc] {
			continue
		}
		if ep, ok := s.reg.EndpointByLocator(loc); ok && ep.Available {
			s.markUnavailable(ep)
			if d, ok := s.reg.GetDevice(ep.DeviceID); ok {
				d.State = domain.DeviceStateOffline
				removed = append(removed, d)
			}
		}
	}
	s.lastSeen = current
	s.mu.Unlock()

	added, err = s.discoverEndpoints(ctx, endpoints)
	return added, removed, err
}

// Watch runs Refresh on an interval until ctx is done, invoking onEvent with
// the added/removed devices each cycle.
func (s *Service) Watch(ctx context.Context, interval time.Duration, onEvent func(added, removed []*domain.Device)) {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			added, removed, err := s.Refresh(ctx)
			if err == nil && onEvent != nil {
				onEvent(added, removed)
			}
		}
	}
}

// discoverEndpoints correlates identity and creates/updates channels for each
// endpoint, deduplicating the returned device list.
func (s *Service) discoverEndpoints(ctx context.Context, endpoints []domain.Endpoint) ([]*domain.Device, error) {
	var found []*domain.Device
	seen := map[domain.DeviceID]bool{}
	for _, ep := range endpoints {
		d, ok := s.discoverEndpoint(ctx, ep)
		if !ok || seen[d.ID] {
			continue
		}
		seen[d.ID] = true
		found = append(found, d)
	}
	return found, nil
}

// discoverEndpoint probes plugins, correlates identity, and creates/updates a
// channel. It is idempotent per locator: re-discovery reuses the existing
// endpoint and channel rather than duplicating them.
func (s *Service) discoverEndpoint(ctx context.Context, ep domain.Endpoint) (*domain.Device, bool) {
	for _, p := range s.plugins.All() {
		res, err := p.Probe(ctx, ep)
		if err != nil || !res.Match {
			continue
		}
		device := s.correlate(res.IdentityHints)

		// Idempotent endpoint: reuse by locator.
		endpoint, exists := s.reg.EndpointByLocator(ep.Locator)
		if !exists {
			ep.DeviceID = device.ID
			for k, v := range res.IdentityHints {
				ep.SetAttr(k, v)
			}
			ep.MarkAvailable()
			s.reg.AddEndpoint(&ep)
			endpoint = &ep
		} else {
			endpoint.MarkAvailable()
			endpoint.DeviceID = device.ID
		}

		channel := s.findChannel(endpoint.ID, p.Info().ID)
		if channel == nil {
			channel = domain.NewChannel(p.Info().ID, res.ChannelType, endpoint.ID, device.ID)
			channel.Locator = ep.Locator
			channel.Cost = res.Cost
			channel.Capabilities = capabilityNames(p.Capabilities(channel))
		}
		wasReady := channel.State == domain.ChannelReady

		// Connect: open the channel and health-check it to READY.
		if err := p.Open(ctx, channel, ""); err != nil {
			channel.State = domain.ChannelFailed
			channel.Healthy = false
			s.reg.AddChannel(channel)
			continue
		}
		s.reg.AddChannel(channel)
		if !wasReady {
			e := domain.NewEvent(domain.EventChannelReady, "core.discovery", "channel")
			e.WithChannel(channel.ID).WithDevice(device.ID)
			s.bus.Publish(e)
		}
		return device, true
	}
	return nil, false
}

// findChannel returns the channel bound to an endpoint for a plugin.
func (s *Service) findChannel(endpointID domain.EndpointID, pluginID string) *domain.Channel {
	for _, ch := range s.reg.ChannelsByEndpoint(endpointID) {
		if ch.PluginID == pluginID {
			return ch
		}
	}
	return nil
}

// markUnavailable marks an endpoint and its channels unavailable and emits
// the hotplug-removal events.
func (s *Service) markUnavailable(ep *domain.Endpoint) {
	ep.MarkUnavailable()
	for _, ch := range s.reg.ChannelsByEndpoint(ep.ID) {
		if ch.State != domain.ChannelReady && ch.State != domain.ChannelDegraded {
			continue
		}
		ch.State = domain.ChannelDegraded
		ch.Healthy = false
		e := domain.NewEvent(domain.EventChannelLost, "core.discovery", "channel")
		e.WithChannel(ch.ID).WithDevice(ep.DeviceID)
		s.bus.Publish(e)
	}
	e := domain.NewEvent(domain.EventEndpointUnavailable, "core.discovery", "endpoint")
	e.WithDevice(ep.DeviceID)
	s.bus.Publish(e)
	off := domain.NewEvent(domain.EventDeviceOffline, "core.discovery", "device")
	off.WithDevice(ep.DeviceID)
	s.bus.Publish(off)
}

// correlate finds or creates a Device from identity hints. The serial is the
// strong identity signal; a serial collision with a DIFFERENT hardware ID is a
// strong conflict and is quarantined, never force-merged (Design doc 05 §10).
func (s *Service) correlate(hints map[string]string) *domain.Device {
	serial := hints["serial"]
	hw := hints["hardware_id"]
	if serial != "" {
		if d, ok := s.reg.DeviceBySerial(serial); ok && d.State != domain.DeviceStateQuarantined {
			if d.HardwareID != "" && hw != "" && d.HardwareID != hw {
				q := domain.NewDevice(serial, hints["model"])
				q.HardwareID = hw
				q.State = domain.DeviceStateQuarantined
				s.reg.UpsertDevice(q)
				s.emitIdentityConflict(serial, d.ID, q.ID)
				return q
			}
			return d
		}
	}
	d := domain.NewDevice(serial, hints["model"])
	d.HardwareID = hw
	if fw := hints["firmware"]; fw != "" {
		d.Properties["firmware"] = fw
	}
	s.reg.UpsertDevice(d)
	return d
}

// emitIdentityConflict publishes a diagnostic event for a strong identity
// conflict (Design doc 05 §10).
func (s *Service) emitIdentityConflict(serial string, existing, quarantined domain.DeviceID) {
	e := domain.NewEvent(domain.EventIdentityConflict, "core.discovery", "identity")
	e.Payload = map[string]string{
		"serial":             serial,
		"existing_device":    string(existing),
		"quarantined_device": string(quarantined),
	}
	s.bus.Publish(e)
}

func capabilityNames(caps []domain.Capability) []domain.CapabilityName {
	names := make([]domain.CapabilityName, 0, len(caps))
	for _, c := range caps {
		names = append(names, c.Name)
	}
	return names
}
