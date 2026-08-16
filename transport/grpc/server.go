// Package grpc exposes the Unified API over gRPC. The Server is a thin
// transport adapter: it contains no business logic, only 1:1 mapping between
// the gRPC ConnectivityService and the in-process sdk.Client. The transport is
// a portable extension over the same abstraction.
package grpc

import (
	"context"
	"errors"
	"net"
	"time"

	"example.com/embedded-loop-channel/api/convert"
	channelv1 "example.com/embedded-loop-channel/api/gen/channelv1"
	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/sdk"

	"google.golang.org/grpc"
)

// Server adapts sdk.Client to the gRPC ConnectivityService.
type Server struct {
	channelv1.UnimplementedConnectivityServiceServer
	client *sdk.Client
}

// NewServer builds a Server over the in-process client.
func NewServer(c *sdk.Client) *Server { return &Server{client: c} }

// Serve starts the gRPC server on addr and blocks until ctx is done. It
// returns the underlying listener address.
func (s *Server) Serve(ctx context.Context, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	gs := grpc.NewServer()
	channelv1.RegisterConnectivityServiceServer(gs, s)

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		gs.GracefulStop()
		close(done)
	}()
	if err := gs.Serve(lis); err != nil {
		return err
	}
	<-done
	return nil
}

// Discover triggers a scan and returns the observed devices.
func (s *Server) Discover(ctx context.Context, _ *channelv1.DiscoverRequest) (*channelv1.DiscoverResponse, error) {
	devices, err := s.client.Discover(ctx)
	if err != nil {
		return &channelv1.DiscoverResponse{Error: errToProto(err)}, nil
	}
	resp := &channelv1.DiscoverResponse{}
	for _, d := range devices {
		resp.Devices = append(resp.Devices, convert.DeviceToProto(d))
	}
	return resp, nil
}

// ListDevices returns all known devices.
func (s *Server) ListDevices(_ context.Context, _ *channelv1.ListDevicesRequest) (*channelv1.ListDevicesResponse, error) {
	devices := s.client.ListDevices()
	resp := &channelv1.ListDevicesResponse{}
	for _, d := range devices {
		resp.Devices = append(resp.Devices, convert.DeviceToProto(d))
	}
	return resp, nil
}

// ListCapabilities returns a device's capability names.
func (s *Server) ListCapabilities(ctx context.Context, req *channelv1.ListCapabilitiesRequest) (*channelv1.ListCapabilitiesResponse, error) {
	caps, err := s.client.ListCapabilities(domain.DeviceID(req.DeviceId))
	if err != nil {
		return &channelv1.ListCapabilitiesResponse{Error: errToProto(err)}, nil
	}
	resp := &channelv1.ListCapabilitiesResponse{}
	for _, c := range caps {
		resp.Capabilities = append(resp.Capabilities, string(c))
	}
	return resp, nil
}

// CreateSession opens a session.
func (s *Server) CreateSession(ctx context.Context, req *channelv1.CreateSessionRequest) (*channelv1.CreateSessionResponse, error) {
	sess, err := s.client.CreateSession(req.Principal, domain.DeviceID(req.DeviceId), time.Duration(req.TtlMs)*time.Millisecond)
	if err != nil {
		return &channelv1.CreateSessionResponse{Error: errToProto(err)}, nil
	}
	return &channelv1.CreateSessionResponse{Session: convert.SessionToProto(sess)}, nil
}

// Execute runs an operation synchronously.
func (s *Server) Execute(ctx context.Context, req *channelv1.ExecuteRequest) (*channelv1.ExecuteResponse, error) {
	result, err := s.client.Execute(ctx, domain.OperationRequest{
		Capability:     domain.CapabilityName(req.Capability),
		Target:         domain.DeviceID(req.Target),
		SessionID:      domain.SessionID(req.SessionId),
		Parameters:     req.Parameters,
		ChannelType:    req.ChannelType,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return &channelv1.ExecuteResponse{Result: convert.OperationResultToProto(result), Error: errToProto(err)}, nil
	}
	return &channelv1.ExecuteResponse{Result: convert.OperationResultToProto(result)}, nil
}

// errToProto converts a Go error to the unified proto Error.
func errToProto(err error) *channelv1.Error {
	var de *domain.Error
	if errors.As(err, &de) {
		return convert.ErrorToProto(de)
	}
	return convert.ErrorToProto(domain.NewError(domain.CodeInternal, domain.CategoryInternal, err.Error()))
}
