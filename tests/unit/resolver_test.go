package unit

import (
	"testing"

	"example.com/embedded-loop-channel/core/registry"
	"example.com/embedded-loop-channel/core/resolver"
	"example.com/embedded-loop-channel/domain"
)

func channelFor(reg *registry.Registry, deviceID domain.DeviceID, pluginID, ctype string, healthy bool, cost int64, caps ...domain.CapabilityName) *domain.Channel {
	ch := domain.NewChannel(pluginID, ctype, domain.EndpointID("ep-"+ctype), deviceID)
	ch.State = domain.ChannelReady
	ch.Healthy = healthy
	ch.Cost = cost
	ch.Capabilities = caps
	reg.AddChannel(ch)
	return ch
}

func TestResolverPrefersHealthyThenCheaper(t *testing.T) {
	reg := registry.New()
	dev := domain.DeviceID("dev-1")

	// Unhealthy channel with a lower cost must lose to a healthy one.
	channelFor(reg, dev, "p1", "adb", false, 5, domain.CapabilityInfoGet)
	healthy := channelFor(reg, dev, "p2", "tcp", true, 10, domain.CapabilityInfoGet)

	r := resolver.New(reg)
	res, err := r.ResolveChannel(dev, domain.CapabilityInfoGet, "")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if res.Channel.ID != healthy.ID {
		t.Fatalf("expected healthy channel, got %s (%s)", res.Channel.ID, res.Reason)
	}

	// With both healthy, the cheaper one wins.
	channelFor(reg, dev, "p3", "uart", true, 2, domain.CapabilityInfoGet)
	res, _ = r.ResolveChannel(dev, domain.CapabilityInfoGet, "")
	if res.Channel.Cost != 2 {
		t.Fatalf("expected cheapest healthy channel, got cost=%d", res.Channel.Cost)
	}
}

func TestResolverChannelTypeOverride(t *testing.T) {
	reg := registry.New()
	dev := domain.DeviceID("dev-1")
	channelFor(reg, dev, "p1", "adb", true, 10, domain.CapabilityInfoGet)
	channelFor(reg, dev, "p2", "tcp", true, 20, domain.CapabilityInfoGet)

	r := resolver.New(reg)
	res, err := r.ResolveChannel(dev, domain.CapabilityInfoGet, "tcp")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if res.Channel.ChannelType != "tcp" {
		t.Fatalf("channel override ignored: got %s", res.Channel.ChannelType)
	}
}

func TestResolverNoCandidate(t *testing.T) {
	reg := registry.New()
	r := resolver.New(reg)
	if _, err := r.ResolveChannel("dev-1", domain.CapabilityFlash, ""); err == nil {
		t.Fatalf("expected error for missing capability")
	}
}
