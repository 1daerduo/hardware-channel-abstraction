package domain

// EndpointType classifies the physical/logical entry point that can be
// discovered and probed.
type EndpointType string

const (
	EndpointUSBADB      EndpointType = "usb-adb"
	EndpointUSBFastboot EndpointType = "usb-fastboot"
	EndpointUART        EndpointType = "uart"
	EndpointJTAG        EndpointType = "jtag"
	EndpointTCP         EndpointType = "tcp"
	EndpointMCP         EndpointType = "mcp"
)

// Endpoint answers "where is it reachable". It is the discoverable entry
// point; a Channel is what you get after you open a connection through it.
type Endpoint struct {
	ID         EndpointID
	DeviceID   DeviceID
	Type       EndpointType
	Locator    string            // e.g. serial port, usb path, tcp addr
	Transport  string            // USB / TCP-IP / Serial / BLE / CAN
	Attributes map[string]string // raw identity hints, probe metadata
	Source     string            // scanner that observed this endpoint
	Available  bool              // false once the endpoint is no longer observed
}

// NewEndpoint builds an Endpoint belonging to device.
func NewEndpoint(deviceID DeviceID, typ EndpointType, locator, transport, source string) *Endpoint {
	return &Endpoint{
		ID:         NewEndpointID(),
		DeviceID:   deviceID,
		Type:       typ,
		Locator:    locator,
		Transport:  transport,
		Attributes: map[string]string{},
		Source:     source,
		Available:  true,
	}
}

// MarkUnavailable records that the endpoint is no longer observed (hotplug
// removal).
func (e *Endpoint) MarkUnavailable() { e.Available = false }

// MarkAvailable records that the endpoint is observed again (hotplug add).
func (e *Endpoint) MarkAvailable() { e.Available = true }

// SetAttr records a discovery/probe attribute (idempotent override).
func (e *Endpoint) SetAttr(k, v string) {
	if e.Attributes == nil {
		e.Attributes = map[string]string{}
	}
	e.Attributes[k] = v
}
