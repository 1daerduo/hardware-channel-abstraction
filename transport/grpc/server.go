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

	"github.com/1daerduo/hardware-channel-abstraction/api/convert"
	channelv1 "github.com/1daerduo/hardware-channel-abstraction/api/gen/channelv1"
	"github.com/1daerduo/hardware-channel-abstraction/batch"
	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/farm"
	"github.com/1daerduo/hardware-channel-abstraction/sdk"

	"google.golang.org/grpc"
)

// Server adapts sdk.Client (and optionally a farm.Scheduler) to gRPC services.
type Server struct {
	channelv1.UnimplementedConnectivityServiceServer
	channelv1.UnimplementedFarmServiceServer
	client    *sdk.Client
	scheduler *farm.Scheduler
}

// NewServer builds a Server over the in-process client.
func NewServer(c *sdk.Client) *Server { return &Server{client: c} }

// WithScheduler attaches a device-farm scheduler, enabling FarmService RPCs.
func (s *Server) WithScheduler(sched *farm.Scheduler) *Server {
	s.scheduler = sched
	return s
}

// Serve starts the gRPC server on addr and blocks until ctx is done.
func (s *Server) Serve(ctx context.Context, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	gs := grpc.NewServer()
	channelv1.RegisterConnectivityServiceServer(gs, s)
	if s.scheduler != nil {
		channelv1.RegisterFarmServiceServer(gs, s)
	}

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

// ---------------------------------------------------------------------------
// FarmService
// ---------------------------------------------------------------------------

func (s *Server) SubmitTask(_ context.Context, req *channelv1.SubmitTaskRequest) (*channelv1.SubmitTaskResponse, error) {
	if s.scheduler == nil {
		return &channelv1.SubmitTaskResponse{Error: errToProto(errors.New("scheduler not configured"))}, nil
	}
	var devices []domain.DeviceID
	for _, d := range req.Devices {
		devices = append(devices, domain.DeviceID(d))
	}
	id, err := s.scheduler.Submit(batch.Request{
		Capability:  domain.CapabilityName(req.Capability),
		Parameters:  req.Parameters,
		Devices:     devices,
		Principal:   req.Principal,
		Concurrency: int(req.Concurrency),
	}, int(req.Priority))
	if err != nil {
		return &channelv1.SubmitTaskResponse{Error: errToProto(err)}, nil
	}
	return &channelv1.SubmitTaskResponse{TaskId: id}, nil
}

func (s *Server) GetTask(_ context.Context, req *channelv1.GetTaskRequest) (*channelv1.GetTaskResponse, error) {
	if s.scheduler == nil {
		return &channelv1.GetTaskResponse{Error: errToProto(errors.New("scheduler not configured"))}, nil
	}
	t, err := s.scheduler.Status(req.TaskId)
	if err != nil {
		return &channelv1.GetTaskResponse{Error: errToProto(err)}, nil
	}
	return &channelv1.GetTaskResponse{Task: taskToProto(t)}, nil
}

func (s *Server) ListTasks(context.Context, *channelv1.ListTasksRequest) (*channelv1.ListTasksResponse, error) {
	resp := &channelv1.ListTasksResponse{}
	if s.scheduler == nil {
		return resp, nil
	}
	for _, t := range s.scheduler.List() {
		resp.Tasks = append(resp.Tasks, taskToProto(t))
	}
	return resp, nil
}

func (s *Server) CancelTask(_ context.Context, req *channelv1.CancelTaskRequest) (*channelv1.CancelTaskResponse, error) {
	if s.scheduler == nil {
		return &channelv1.CancelTaskResponse{Error: errToProto(errors.New("scheduler not configured"))}, nil
	}
	if err := s.scheduler.Cancel(req.TaskId); err != nil {
		return &channelv1.CancelTaskResponse{Error: errToProto(err)}, nil
	}
	return &channelv1.CancelTaskResponse{}, nil
}

func (s *Server) ListPool(context.Context, *channelv1.ListPoolRequest) (*channelv1.ListPoolResponse, error) {
	resp := &channelv1.ListPoolResponse{}
	if s.scheduler == nil {
		return resp, nil
	}
	for _, e := range s.scheduler.PoolSnapshot() {
		entry := &channelv1.PoolEntry{
			DeviceId:    string(e.Device.ID),
			Serial:      e.Device.Serial,
			Busy:        e.Busy,
			CurrentTask: e.CurrentTask,
			LastState:   string(e.LastState),
		}
		if e.LastError != nil {
			entry.LastError = convert.ErrorToProto(e.LastError)
		}
		resp.Entries = append(resp.Entries, entry)
	}
	return resp, nil
}

func taskToProto(t *farm.Task) *channelv1.Task {
	p := &channelv1.Task{
		Id:            t.ID,
		State:         string(t.State),
		Priority:      int32(t.Priority),
		Capability:    string(t.Request.Capability),
		Err:           t.Err,
		SubmittedAtMs: t.SubmittedAt.UnixMilli(),
		StartedAtMs:   t.StartedAt.UnixMilli(),
		CompletedAtMs: t.CompletedAt.UnixMilli(),
	}
	if t.Summary != nil {
		p.Total = int64(t.Summary.Total)
		p.Succeeded = int64(t.Summary.Succeeded)
		p.Failed = int64(t.Summary.Failed)
	}
	return p
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

// DescribeCapabilities returns full capability descriptors.
func (s *Server) DescribeCapabilities(_ context.Context, req *channelv1.ListCapabilitiesRequest) (*channelv1.DescribeCapabilitiesResponse, error) {
	caps, err := s.client.DescribeCapabilities(domain.DeviceID(req.DeviceId))
	if err != nil {
		return &channelv1.DescribeCapabilitiesResponse{Error: errToProto(err)}, nil
	}
	resp := &channelv1.DescribeCapabilitiesResponse{}
	for i := range caps {
		resp.Capabilities = append(resp.Capabilities, convert.CapabilityToProto(&caps[i]))
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
