// Package sdk is the stable developer SDK for Channel Plugins. It defines the
// Plugin SPI contract (Design docs 04, 12, 16). Plugins depend on this
// package and the domain model; they must never depend on core internals.
package sdk

import (
	"fmt"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// TrustLevel participates in Policy decisions but never replaces Capability
// Permission (Design doc 10).
type TrustLevel string

const (
	TrustTrusted   TrustLevel = "TRUSTED"
	TrustVerified  TrustLevel = "VERIFIED"
	TrustUntrusted TrustLevel = "UNTRUSTED"
)

// Manifest is the declarative metadata a Plugin must provide. The ID is
// stable and globally unique; version changes never rename the ID.
type Manifest struct {
	ID              string
	Version         string
	APIVersion      string
	Protocol        string
	Capabilities    []domain.CapabilityName
	Transports      []string
	RecoveryActions []string
	TrustLevel      TrustLevel
}

// Validate enforces the Manifest contract (Design doc 16): a stable ID, an
// API version, at least one capability and a known trust level.
func (m Manifest) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("manifest: id is required")
	}
	if m.APIVersion == "" {
		return fmt.Errorf("manifest %s: api_version is required", m.ID)
	}
	if len(m.Capabilities) == 0 {
		return fmt.Errorf("manifest %s: at least one capability is required", m.ID)
	}
	switch m.TrustLevel {
	case TrustTrusted, TrustVerified, TrustUntrusted:
	default:
		return fmt.Errorf("manifest %s: unknown trust level %q", m.ID, m.TrustLevel)
	}
	return nil
}
