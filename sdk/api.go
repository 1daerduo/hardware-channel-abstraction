package sdk

import (
	"context"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// ConnectivityAPI is the stable Unified API contract (Design doc 03). It is
// the abstraction a consumer programs against: whether the implementation is
// in-process (*Client) or a remote transport (grpc.Client), the code is
// identical. Adding a new transport is a portable extension that never
// touches this contract.
type ConnectivityAPI interface {
	Discover(ctx context.Context) ([]*domain.Device, error)
	ListDevices() []*domain.Device
	ListCapabilities(deviceID domain.DeviceID) ([]domain.CapabilityName, error)
	DescribeCapabilities(deviceID domain.DeviceID) ([]domain.Capability, error)
	CreateSession(principal string, deviceID domain.DeviceID, ttl time.Duration) (*domain.Session, error)
	Execute(ctx context.Context, req domain.OperationRequest) (*domain.OperationResult, error)
}

// Compile-time assertions: the in-process client satisfies the contract.
var _ ConnectivityAPI = (*Client)(nil)
