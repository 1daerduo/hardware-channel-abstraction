package grpc

import (
	"context"
	"time"

	"example.com/embedded-loop-channel/api/convert"
	channelv1 "example.com/embedded-loop-channel/api/gen/channelv1"
	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/sdk"

	"google.golang.org/grpc"
)

// Client is a remote implementation of sdk.ConnectivityAPI: it delegates every
// method to the gRPC ConnectivityService. A consumer writes code against the
// ConnectivityAPI interface and is indifferent to whether the implementation
// is in-process or remote.
type Client struct {
	grpc channelv1.ConnectivityServiceClient
}

// NewClient builds a remote client over a gRPC connection.
func NewClient(conn grpc.ClientConnInterface) *Client {
	return &Client{grpc: channelv1.NewConnectivityServiceClient(conn)}
}

var _ sdk.ConnectivityAPI = (*Client)(nil)

// Discover triggers a remote scan.
func (c *Client) Discover(ctx context.Context) ([]*domain.Device, error) {
	resp, err := c.grpc.Discover(ctx, &channelv1.DiscoverRequest{})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, convert.ErrorFromProto(resp.Error)
	}
	var out []*domain.Device
	for _, d := range resp.Devices {
		out = append(out, convert.DeviceFromProto(d))
	}
	return out, nil
}

// ListDevices returns all known devices.
func (c *Client) ListDevices() []*domain.Device {
	resp, err := c.grpc.ListDevices(context.Background(), &channelv1.ListDevicesRequest{})
	if err != nil || resp.Error != nil {
		return nil
	}
	var out []*domain.Device
	for _, d := range resp.Devices {
		out = append(out, convert.DeviceFromProto(d))
	}
	return out
}

// ListCapabilities returns a device's capability names.
func (c *Client) ListCapabilities(deviceID domain.DeviceID) ([]domain.CapabilityName, error) {
	resp, err := c.grpc.ListCapabilities(context.Background(), &channelv1.ListCapabilitiesRequest{DeviceId: string(deviceID)})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, convert.ErrorFromProto(resp.Error)
	}
	var out []domain.CapabilityName
	for _, cap := range resp.Capabilities {
		out = append(out, domain.CapabilityName(cap))
	}
	return out, nil
}

// CreateSession opens a session.
func (c *Client) CreateSession(principal string, deviceID domain.DeviceID, ttl time.Duration) (*domain.Session, error) {
	resp, err := c.grpc.CreateSession(context.Background(), &channelv1.CreateSessionRequest{
		Principal: principal,
		DeviceId:  string(deviceID),
		TtlMs:     ttl.Milliseconds(),
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, convert.ErrorFromProto(resp.Error)
	}
	sess := convert.SessionFromProto(resp.Session)
	return sess, nil
}

// Execute runs an operation remotely.
func (c *Client) Execute(ctx context.Context, req domain.OperationRequest) (*domain.OperationResult, error) {
	resp, err := c.grpc.Execute(ctx, &channelv1.ExecuteRequest{
		Capability:     string(req.Capability),
		Target:         string(req.Target),
		SessionId:      string(req.SessionID),
		Parameters:     req.Parameters,
		ChannelType:    req.ChannelType,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return convert.OperationResultFromProto(resp.Result), convert.ErrorFromProto(resp.Error)
	}
	return convert.OperationResultFromProto(resp.Result), nil
}
