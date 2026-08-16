// Package resolver selects a Channel for a (device, capability) request
// (Design docs 05 §14, 12 §29). It ranks candidates deterministically by
// health and cost and records the selection reason.
package resolver

import (
	"fmt"
	"sort"
	"strings"

	"example.com/embedded-loop-channel/core/registry"
	"example.com/embedded-loop-channel/domain"
)

// Resolver chooses channels from the Registry.
type Resolver struct {
	reg *registry.Registry
}

// New builds a Resolver over reg.
func New(reg *registry.Registry) *Resolver { return &Resolver{reg: reg} }

// Resolution records the chosen channel and why it was chosen.
type Resolution struct {
	Channel    *domain.Channel
	Reason     string
	Candidates []domain.ChannelID
}

// ResolveChannel returns the best READY channel for capability, optionally
// constrained to a channel type (channel override). Ranking is deterministic:
// healthy first, then lower cost, then stable channel ID.
func (r *Resolver) ResolveChannel(deviceID domain.DeviceID, capability domain.CapabilityName, channelType string) (*Resolution, error) {
	cands := r.reg.ChannelsByDeviceCapability(deviceID, capability)
	if channelType != "" {
		var filtered []*domain.Channel
		for _, c := range cands {
			if c.ChannelType == channelType {
				filtered = append(filtered, c)
			}
		}
		cands = filtered
	}
	if len(cands) == 0 {
		return nil, &domain.Error{
			Code:        domain.CodeUnsupportedCap,
			Category:    domain.CategoryDiscovery,
			Message:     fmt.Sprintf("no ready channel for capability %s", capability),
			Recoverable: true,
			Details:     map[string]string{"device_id": string(deviceID)},
		}
	}

	res := &Resolution{}
	for _, c := range cands {
		res.Candidates = append(res.Candidates, c.ID)
	}

	sort.SliceStable(cands, func(i, j int) bool {
		hi, hj := cands[i].Healthy, cands[j].Healthy
		if hi != hj {
			return hi
		}
		if cands[i].Cost != cands[j].Cost {
			return cands[i].Cost < cands[j].Cost
		}
		return cands[i].ID < cands[j].ID
	})

	res.Channel = cands[0]
	var parts []string
	parts = append(parts, fmt.Sprintf("healthy=%t", res.Channel.Healthy))
	parts = append(parts, fmt.Sprintf("cost=%d", res.Channel.Cost))
	parts = append(parts, fmt.Sprintf("channel_type=%s", res.Channel.ChannelType))
	res.Reason = "selected " + string(res.Channel.ID) + " (" + strings.Join(parts, ", ") + ")"
	return res, nil
}
