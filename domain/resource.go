package domain

import "time"

// LockMode is the concurrency mode of a lease. The MVP implements SHARED and
// EXCLUSIVE.
type LockMode string

const (
	LockShared    LockMode = "SHARED"
	LockExclusive LockMode = "EXCLUSIVE"
)

// Standard resource types. Capabilities declare these as requirements; the
// Resource Registry maps a type to the concrete Resource instance of a device.
const (
	ResourceTypeDevice     = "device"
	ResourceTypeFlash      = "flash"
	ResourceTypeDebug      = "debug"
	ResourceTypeConsole    = "console"
	ResourceTypeFilesystem = "filesystem"
)

// Resource is a governed object: a subsystem, channel, flash partition or
// filesystem that operations contend over.
type Resource struct {
	ID       ResourceID
	Type     string // flash / debug / console / filesystem / device
	DeviceID DeviceID
	State    string
}

// NewResource builds a Resource.
func NewResource(typ string, deviceID DeviceID) *Resource {
	return &Resource{
		ID:       NewResourceID(),
		Type:     typ,
		DeviceID: deviceID,
		State:    "AVAILABLE",
	}
}

// Lease is the ownership record with an automatic expiry (TTL).
type Lease struct {
	ID         LeaseID
	ResourceID ResourceID
	Holder     string
	Mode       LockMode
	ExpiresAt  time.Time
}

// Expired reports whether the lease has lapsed.
func (l *Lease) Expired(now time.Time) bool { return now.After(l.ExpiresAt) }
