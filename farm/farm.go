// Package farm is the device-farm scheduler: a priority task queue with a
// resident worker pool, plus a live device-pool state. It is a consumer ABOVE
// the Connectivity Core (Design doc 11 §19): the scheduler owns task
// ordering/concurrency, the Core owns per-Operation reliability.
package farm

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"example.com/embedded-loop-channel/batch"
	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/sdk"
)

// TaskState is the lifecycle of a queued task.
type TaskState string

const (
	TaskPending   TaskState = "PENDING"
	TaskRunning   TaskState = "RUNNING"
	TaskSucceeded TaskState = "SUCCEEDED"
	TaskFailed    TaskState = "FAILED"
	TaskCancelled TaskState = "CANCELLED"
)

// Task is one queued batch operation.
type Task struct {
	ID          string
	Request     batch.Request
	Priority    int
	State       TaskState
	Summary     *batch.Summary
	Err         string
	SubmittedAt time.Time
	StartedAt   time.Time
	CompletedAt time.Time
}

// PoolEntry is the live state of one device in the farm.
type PoolEntry struct {
	Device      *domain.Device
	Busy        bool
	CurrentTask string
	LastState   domain.OperationState
	LastError   *domain.Error
}

// Scheduler runs a priority queue of batch tasks with a resident worker pool,
// and maintains the device pool state.
type Scheduler struct {
	api     sdk.ConnectivityAPI
	workers int

	mu     sync.Mutex
	tasks  map[string]*Task
	order  []string
	nextID uint64
	pool   map[domain.DeviceID]*PoolEntry

	ready    chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// New builds a Scheduler with a worker count (default 4).
func New(api sdk.ConnectivityAPI, workers int) *Scheduler {
	if workers <= 0 {
		workers = 4
	}
	return &Scheduler{
		api:     api,
		workers: workers,
		tasks:   map[string]*Task{},
		pool:    map[domain.DeviceID]*PoolEntry{},
		ready:   make(chan struct{}, 1),
		stop:    make(chan struct{}),
	}
}

// Start launches the resident worker pool.
func (s *Scheduler) Start() {
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker()
	}
}

// Stop shuts down the worker pool (drains in-flight tasks).
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
	s.wg.Wait()
}

// Submit enqueues a batch request with a priority (higher runs first).
func (s *Scheduler) Submit(req batch.Request, priority int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("task-%d", s.nextID)
	s.nextID++
	s.tasks[id] = &Task{
		ID: id, Request: req, Priority: priority, State: TaskPending, SubmittedAt: time.Now(),
	}
	s.order = append(s.order, id)
	s.signal()
	return id, nil
}

// Status returns a task by ID.
func (s *Scheduler) Status(id string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %s not found", id)
	}
	return t, nil
}

// List returns all tasks in submission order.
func (s *Scheduler) List() []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Task, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.tasks[id])
	}
	return out
}

// Cancel cancels a PENDING task.
func (s *Scheduler) Cancel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	if t.State != TaskPending {
		return fmt.Errorf("task %s is %s, cannot cancel", id, t.State)
	}
	t.State = TaskCancelled
	t.CompletedAt = time.Now()
	return nil
}

// PoolSnapshot returns the live device-pool state, ordered by serial.
func (s *Scheduler) PoolSnapshot() []PoolEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PoolEntry, 0, len(s.pool))
	for _, e := range s.pool {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Device.Serial < out[j].Device.Serial })
	return out
}

// worker loops: dequeue the highest-priority pending task, run it, repeat.
func (s *Scheduler) worker() {
	defer s.wg.Done()
	for {
		t := s.dequeue()
		if t == nil {
			return
		}
		s.runTask(t)
	}
}

// dequeue returns the next pending task (highest priority, then FIFO), or nil
// when stopped.
func (s *Scheduler) dequeue() *Task {
	for {
		s.mu.Lock()
		var best *Task
		for _, id := range s.order {
			t := s.tasks[id]
			if t.State == TaskPending && (best == nil || t.Priority > best.Priority) {
				best = t
			}
		}
		if best != nil {
			best.State = TaskRunning
			best.StartedAt = time.Now()
			s.mu.Unlock()
			return best
		}
		s.mu.Unlock()
		select {
		case <-s.ready:
		case <-s.stop:
			return nil
		}
	}
}

func (s *Scheduler) signal() {
	select {
	case s.ready <- struct{}{}:
	default:
	}
}

// runTask executes one task and updates the device pool.
func (s *Scheduler) runTask(t *Task) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.markBusy(t)
	sum, err := batch.New(s.api).Run(ctx, t.Request)
	s.markDone(t, sum, err)
}

// markBusy marks the task's target devices busy.
func (s *Scheduler) markBusy(t *Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.resolveDevicesLocked(t.Request) {
		e := s.pool[d.ID]
		if e == nil {
			e = &PoolEntry{Device: d}
			s.pool[d.ID] = e
		} else {
			e.Device = d
		}
		e.Busy = true
		e.CurrentTask = t.ID
	}
}

// markDone records the task outcome and clears busy on its devices.
func (s *Scheduler) markDone(t *Task, sum *batch.Summary, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.CompletedAt = time.Now()
	if err != nil {
		t.State = TaskFailed
		t.Err = err.Error()
	} else {
		t.State = TaskSucceeded
		t.Summary = sum
	}
	if sum != nil {
		for _, r := range sum.Results {
			e := s.pool[r.Device.ID]
			if e == nil {
				e = &PoolEntry{Device: r.Device}
				s.pool[r.Device.ID] = e
			}
			e.Busy = false
			e.CurrentTask = ""
			e.LastState = r.State
			e.LastError = r.Error
		}
	}
}

// resolveDevicesLocked returns the target devices (caller holds s.mu).
func (s *Scheduler) resolveDevicesLocked(req batch.Request) []*domain.Device {
	all := s.api.ListDevices()
	if len(all) == 0 {
		all, _ = s.api.Discover(context.Background())
	}
	if len(req.Devices) == 0 {
		return all
	}
	wanted := map[string]bool{}
	for _, id := range req.Devices {
		wanted[string(id)] = true
	}
	var out []*domain.Device
	for _, d := range all {
		if wanted[string(d.ID)] || wanted[d.Serial] {
			out = append(out, d)
		}
	}
	return out
}
