package grpc

import (
	"context"

	"example.com/embedded-loop-channel/api/convert"
	channelv1 "example.com/embedded-loop-channel/api/gen/channelv1"
	"example.com/embedded-loop-channel/batch"

	"google.golang.org/grpc"
)

// TaskView is the client-facing view of a farm task.
type TaskView struct {
	ID               string
	State            string
	Priority         int
	Capability       string
	Total, Succeeded int
	Failed           int
	Err              string
}

// PoolEntryView is the client-facing view of one device in the pool.
type PoolEntryView struct {
	DeviceID    string
	Serial      string
	Busy        bool
	CurrentTask string
	LastState   string
	LastError   string
}

// FarmClient is a remote scheduler client: it submits/reads tasks and inspects
// the device pool over the FarmService RPCs.
type FarmClient struct {
	grpc channelv1.FarmServiceClient
}

// NewFarmClient builds a FarmClient over a gRPC connection.
func NewFarmClient(conn grpc.ClientConnInterface) *FarmClient {
	return &FarmClient{grpc: channelv1.NewFarmServiceClient(conn)}
}

// Submit enqueues a batch request and returns the task id.
func (c *FarmClient) Submit(ctx context.Context, req batch.Request, priority int) (string, error) {
	devices := make([]string, 0, len(req.Devices))
	for _, d := range req.Devices {
		devices = append(devices, string(d))
	}
	resp, err := c.grpc.SubmitTask(ctx, &channelv1.SubmitTaskRequest{
		Capability:  string(req.Capability),
		Parameters:  req.Parameters,
		Devices:     devices,
		Principal:   req.Principal,
		Priority:    int32(priority),
		Concurrency: int32(req.Concurrency),
	})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", convert.ErrorFromProto(resp.Error)
	}
	return resp.TaskId, nil
}

// Status returns a task's status view.
func (c *FarmClient) Status(ctx context.Context, id string) (TaskView, error) {
	resp, err := c.grpc.GetTask(ctx, &channelv1.GetTaskRequest{TaskId: id})
	if err != nil {
		return TaskView{}, err
	}
	if resp.Error != nil {
		return TaskView{}, convert.ErrorFromProto(resp.Error)
	}
	return taskView(resp.Task), nil
}

// List returns all tasks.
func (c *FarmClient) List(ctx context.Context) ([]TaskView, error) {
	resp, err := c.grpc.ListTasks(ctx, &channelv1.ListTasksRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]TaskView, 0, len(resp.Tasks))
	for _, t := range resp.Tasks {
		out = append(out, taskView(t))
	}
	return out, nil
}

// Cancel cancels a pending task.
func (c *FarmClient) Cancel(ctx context.Context, id string) error {
	resp, err := c.grpc.CancelTask(ctx, &channelv1.CancelTaskRequest{TaskId: id})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return convert.ErrorFromProto(resp.Error)
	}
	return nil
}

// Pool returns the device pool snapshot.
func (c *FarmClient) Pool(ctx context.Context) ([]PoolEntryView, error) {
	resp, err := c.grpc.ListPool(ctx, &channelv1.ListPoolRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]PoolEntryView, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		le := ""
		if e.LastError != nil {
			le = e.LastError.Message
		}
		out = append(out, PoolEntryView{
			DeviceID:    e.DeviceId,
			Serial:      e.Serial,
			Busy:        e.Busy,
			CurrentTask: e.CurrentTask,
			LastState:   e.LastState,
			LastError:   le,
		})
	}
	return out, nil
}

func taskView(t *channelv1.Task) TaskView {
	if t == nil {
		return TaskView{}
	}
	return TaskView{
		ID:         t.Id,
		State:      t.State,
		Priority:   int(t.Priority),
		Capability: t.Capability,
		Total:      int(t.Total),
		Succeeded:  int(t.Succeeded),
		Failed:     int(t.Failed),
		Err:        t.Err,
	}
}
