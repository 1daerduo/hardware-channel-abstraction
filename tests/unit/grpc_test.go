package unit

import (
	"context"
	"net"
	"testing"
	"time"

	channelv1 "github.com/1daerduo/hardware-channel-abstraction/api/gen/channelv1"
	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/runtime"
	"github.com/1daerduo/hardware-channel-abstraction/sdk"
	grpctransport "github.com/1daerduo/hardware-channel-abstraction/transport/grpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestGRPCConnectivityAPI verifies the Unified API works over the wire: a
// remote gRPC client (a sdk.ConnectivityAPI) discovers, lists capabilities,
// opens a session and executes an operation against the in-process core.
func TestGRPCConnectivityAPI(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2"),
	))
	rt.Client.Grant("agent", domain.CapabilityInfoGet)
	rt.Client.PreApprove("agent", domain.CapabilityInfoGet)

	// Start the gRPC server on a random local port.
	srv := grpctransport.NewServer(rt.Client)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	channelv1.RegisterConnectivityServiceServer(gs, srv)
	go gs.Serve(lis)
	defer gs.Stop()

	// Connect a remote client.
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	remote := grpctransport.NewClient(conn)

	// The remote client satisfies the SAME abstraction as the in-process one.
	var _ sdk.ConnectivityAPI = remote

	devices, err := remote.Discover(ctx)
	if err != nil || len(devices) != 1 {
		t.Fatalf("discover: devices=%d err=%v", len(devices), err)
	}

	caps, err := remote.ListCapabilities(devices[0].ID)
	if err != nil {
		t.Fatalf("list capabilities: %v", err)
	}
	if !containsCap(caps, domain.CapabilityInfoGet) {
		t.Fatalf("capabilities missing info.get: %v", caps)
	}

	sess, err := remote.CreateSession("agent", devices[0].ID, time.Minute)
	if err != nil || sess == nil {
		t.Fatalf("create session: %v", err)
	}

	res, err := remote.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: devices[0].ID, SessionID: sess.ID,
	})
	if err != nil || res == nil || res.State != domain.OperationSucceeded {
		t.Fatalf("execute: res=%+v err=%v", res, err)
	}
}

func containsCap(caps []domain.CapabilityName, want domain.CapabilityName) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}
