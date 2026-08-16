package domain

// ChannelState is the runtime lifecycle of a Channel, managed by Core; the
// Plugin only performs the underlying protocol actions.
type ChannelState string

const (
	ChannelUnresolved   ChannelState = "UNRESOLVED"
	ChannelResolving    ChannelState = "RESOLVING"
	ChannelOpening      ChannelState = "OPENING"
	ChannelReady        ChannelState = "READY"
	ChannelDegraded     ChannelState = "DEGRADED"
	ChannelReconnecting ChannelState = "RECONNECTING"
	ChannelClosed       ChannelState = "CLOSED"
	ChannelFailed       ChannelState = "FAILED"
)

// Channel is the runtime instance of a connection through an Endpoint. A
// Plugin is a code type; a Channel is a concrete, stateful instance.
type Channel struct {
	ID           ChannelID
	PluginID     string
	ChannelType  string
	EndpointID   EndpointID
	DeviceID     DeviceID
	Locator      string // endpoint locator (set by Discovery before Open)
	State        ChannelState
	Healthy      bool
	Cost         int64
	Capabilities []CapabilityName
}

// NewChannel builds a Channel in the OPENING state; Core moves it to READY
// after a successful open + health check.
func NewChannel(pluginID, channelType string, endpointID EndpointID, deviceID DeviceID) *Channel {
	return &Channel{
		ID:           NewChannelID(),
		PluginID:     pluginID,
		ChannelType:  channelType,
		EndpointID:   endpointID,
		DeviceID:     deviceID,
		State:        ChannelOpening,
		Healthy:      false,
		Capabilities: []CapabilityName{},
	}
}

// Supports reports whether this Channel advertises the given capability.
func (c *Channel) Supports(name CapabilityName) bool {
	for _, cap := range c.Capabilities {
		if cap == name {
			return true
		}
	}
	return false
}
