package unit

import (
	"context"
	"net"
	"testing"
	"time"

	channelv1 "github.com/1daerduo/hardware-channel-abstraction/api/gen/channelv1"
	"github.com/1daerduo/hardware-channel-abstraction/batch"
	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/farm"
	"github.com/1daerduo/hardware-channel-abstraction/runtime"
	grpctransport "github.com/1daerduo/hardware-channel-abstraction/transport/grpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestFarmServiceOverGRPC verifies the device-farm scheduler is reachable
// over gRPC: submit a task, poll its status, and inspect the device pool.
func TestFarmServiceOverGRPC(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("DEV-1", "board", "1.0", "usb:1-1.1"),
		fake.NewDevice("DEV-2", "board", "1.0", "usb:1-1.2"),
		fake.NewDevice("DEV-3", "board", "1.0", "usb:1-1.3"),
	))
	defer rt.Close()
	rt.Client.Grant("farm", domain.CapabilityInfoGet)
	rt.Client.PreApprove("farm", domain.CapabilityInfoGet)
	rt.Client.Discover(ctx)

	sched := farm.New(rt.Client, 2)
	sched.Start()
	defer sched.Stop()

	srv := grpctransport.NewServer(rt.Client).WithScheduler(sched)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	channelv1.RegisterConnectivityServiceServer(gs, srv)
	channelv1.RegisterFarmServiceServer(gs, srv)
	go gs.Serve(lis)
	defer gs.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	fc := grpctransport.NewFarmClient(conn)

	id, err := fc.Submit(ctx, batch.Request{
		Capability: domain.CapabilityInfoGet, Principal: "farm",
	}, 0)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Poll status until SUCCEEDED.
	deadline := time.Now().Add(3 * time.Second)
	for {
		view, err := fc.Status(ctx, id)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if view.State == "SUCCEEDED" {
			if view.Succeeded != 3 || view.Failed != 0 {
				t.Fatalf("task view = %+v", view)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task did not succeed: %+v", view)
		}
		time.Sleep(20 * time.Millisecond)
	}

	pool, err := fc.Pool(ctx)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if len(pool) != 3 {
		t.Fatalf("pool size = %d, want 3", len(pool))
	}
	for _, e := range pool {
		if e.LastState != "SUCCEEDED" || e.Busy {
			t.Fatalf("pool entry = %+v", e)
		}
	}
}
