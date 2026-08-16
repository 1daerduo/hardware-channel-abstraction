// Package batch is the device-farm orchestration head: it runs one capability
// across many devices concurrently and aggregates per-device results.
//
// It is a consumer ABOVE the Connectivity Core (Design doc 11 §33-34): the
// batch layer owns concurrency/batching/aggregation, while the Core owns the
// reliability of each individual Operation. The Executor programs only against
// sdk.ConnectivityAPI, so it works in-process or over gRPC unchanged.
package batch

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/sdk"
)

// Request is one batch operation: a capability + parameters applied to a set
// of devices.
type Request struct {
	Capability  domain.CapabilityName
	Parameters  map[string]string
	Devices     []domain.DeviceID // empty = all known devices
	Principal   string            // default "batch"
	SessionTTL  time.Duration     // default 1m
	Concurrency int               // 0 = one per device
}

// Result is the outcome for one device.
type Result struct {
	Device *domain.Device
	State  domain.OperationState
	Output string
	Error  *domain.Error
}

// Summary aggregates a batch run.
type Summary struct {
	Total     int
	Succeeded int
	Failed    int
	Results   []Result // in the same order as the resolved device list
}

// Executor runs batch operations over a ConnectivityAPI.
type Executor struct {
	api sdk.ConnectivityAPI
}

// New builds an Executor over api.
func New(api sdk.ConnectivityAPI) *Executor { return &Executor{api: api} }

// Run executes the capability on every resolved device concurrently, bounded
// by Concurrency, and returns the aggregated summary. ctx cancels the whole
// batch (in-flight operations observe it).
func (e *Executor) Run(ctx context.Context, req Request) (*Summary, error) {
	devices, err := e.resolveDevices(req)
	if err != nil {
		return nil, err
	}
	if req.Principal == "" {
		req.Principal = "batch"
	}
	if req.SessionTTL <= 0 {
		req.SessionTTL = time.Minute
	}
	conc := req.Concurrency
	if conc <= 0 || conc > len(devices) {
		conc = len(devices)
	}

	results := make([]Result, len(devices))
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup

	for i, d := range devices {
		wg.Add(1)
		go func(i int, d *domain.Device) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[i] = Result{Device: d, State: domain.OperationCancelled,
					Error: domain.NewError(domain.CodeUnknown, domain.CategoryCancellation, "cancelled")}
				return
			}

			sess, err := e.api.CreateSession(req.Principal, d.ID, req.SessionTTL)
			if err != nil {
				results[i] = Result{Device: d, State: domain.OperationResourceFailed, Error: asDomainErr(err)}
				return
			}
			res, err := e.api.Execute(ctx, domain.OperationRequest{
				Capability: req.Capability, Target: d.ID, SessionID: sess.ID, Parameters: req.Parameters,
			})
			results[i] = Result{Device: d, State: res.State, Output: res.Output, Error: asDomainErr(err)}
		}(i, d)
	}
	wg.Wait()

	sum := &Summary{Total: len(devices), Results: results}
	for _, r := range results {
		if r.State == domain.OperationSucceeded {
			sum.Succeeded++
		} else {
			sum.Failed++
		}
	}
	return sum, nil
}

// resolveDevices returns the target devices: all known devices, or the subset
// matched by ID/serial.
func (e *Executor) resolveDevices(req Request) ([]*domain.Device, error) {
	all := e.api.ListDevices()
	if len(all) == 0 {
		all, _ = e.api.Discover(context.Background())
	}
	if len(req.Devices) == 0 {
		return all, nil
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
	return out, nil
}

func asDomainErr(err error) *domain.Error {
	if err == nil {
		return nil
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return de
	}
	return domain.NewError(domain.CodeInternal, domain.CategoryInternal, err.Error())
}
