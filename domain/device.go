package domain

import "time"

// DeviceState is the business-level state of a Device. It is deliberately
// separate from Endpoint reachability and Channel health.
type DeviceState string

const (
	DeviceStateUnspecified DeviceState = ""
	DeviceStateOnline      DeviceState = "ONLINE"
	DeviceStateOffline     DeviceState = "OFFLINE"
	DeviceStateRecovering  DeviceState = "RECOVERING"
	DeviceStateQuarantined DeviceState = "QUARANTINED"
)

// Device answers "who is this thing". It correlates one or more Endpoints
// that share a stable identity.
//
// HardwareID is the strong hardware-unique identity signal (Design doc 05 §9).
// A serial collision with a DIFFERENT HardwareID means two physically distinct
// boards claim the same serial — a strong conflict that must be quarantined,
// never force-merged.
type Device struct {
	ID         DeviceID
	Serial     string
	HardwareID string
	Model      string
	State      DeviceState
	Properties map[string]string
	Endpoints  []EndpointID
	ObservedAt time.Time
}

// NewDevice builds a Device with the given serial/model. The serial is the
// strong identity signal but never the sole physical address of the device.
func NewDevice(serial, model string) *Device {
	return &Device{
		ID:         NewDeviceID(),
		Serial:     serial,
		Model:      model,
		State:      DeviceStateOnline,
		Properties: map[string]string{},
		ObservedAt: time.Now(),
	}
}

// AddEndpoint records an Endpoint that belongs to this Device (idempotent).
func (d *Device) AddEndpoint(id EndpointID) {
	for _, e := range d.Endpoints {
		if e == id {
			return
		}
	}
	d.Endpoints = append(d.Endpoints, id)
}
