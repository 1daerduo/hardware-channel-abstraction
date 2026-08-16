package unit

import (
	"testing"

	"github.com/1daerduo/hardware-channel-abstraction/api/convert"
	channelv1 "github.com/1daerduo/hardware-channel-abstraction/api/gen/channelv1"
	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"google.golang.org/protobuf/proto"
)

// TestProtoRoundtrip verifies the canonical wire contract survives
// serialization (Design doc 14 §19: serialization roundtrip).
func TestProtoRoundtrip(t *testing.T) {
	dev := &domain.Device{
		ID:         domain.DeviceID("dev-1"),
		Serial:     "ABC123",
		Model:      "eval-board",
		State:      domain.DeviceStateOnline,
		Properties: map[string]string{"firmware": "1.2.3"},
		Endpoints:  []domain.EndpointID{"ep-1", "ep-2"},
	}

	pb := convert.DeviceToProto(dev)
	bytes, err := proto.Marshal(pb)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var back channelv1.Device
	if err := proto.Unmarshal(bytes, &back); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	restored := convert.DeviceFromProto(&back)
	if restored.ID != dev.ID || restored.Serial != dev.Serial || restored.Model != dev.Model {
		t.Fatalf("roundtrip mismatch: %+v", restored)
	}
	if restored.State != domain.DeviceStateOnline {
		t.Fatalf("state enum roundtrip failed: %s", restored.State)
	}
	if len(restored.Endpoints) != 2 {
		t.Fatalf("endpoints roundtrip failed: %d", len(restored.Endpoints))
	}
}

// TestErrorRoundtrip verifies the unified error taxonomy survives conversion.
func TestErrorRoundtrip(t *testing.T) {
	orig := domain.NewError(domain.CodeDeviceOffline, domain.CategoryDeviceState, "device offline").
		WithDetail("serial", "ABC123")
	orig.Retryable = false
	orig.Recoverable = true

	pb := convert.ErrorToProto(orig)
	back := convert.ErrorFromProto(pb)

	if back.Code != orig.Code || back.Category != orig.Category || back.Recoverable != orig.Recoverable {
		t.Fatalf("error roundtrip mismatch: %+v", back)
	}
	if back.Details["serial"] != "ABC123" {
		t.Fatalf("error details lost: %+v", back.Details)
	}
}

// TestOperationStateEnumCoverage ensures every domain state maps to proto and
// back (guards against silently dropping new states).
func TestOperationStateEnumCoverage(t *testing.T) {
	states := []domain.OperationState{
		domain.OperationCreated, domain.OperationValidating, domain.OperationWaitingResource,
		domain.OperationResolving, domain.OperationRunning, domain.OperationVerifying,
		domain.OperationSucceeded, domain.OperationValidationFailed, domain.OperationResourceFailed,
		domain.OperationResolveFailed, domain.OperationRunFailed, domain.OperationVerifyFailed,
		domain.OperationCancelled, domain.OperationTimedOut, domain.OperationUnknown,
	}
	for _, s := range states {
		r := &domain.OperationResult{State: s}
		pb := convert.OperationResultToProto(r)
		if pb.State == channelv1.OperationState_OPERATION_STATE_UNSPECIFIED {
			t.Fatalf("state %s mapped to UNSPECIFIED", s)
		}
	}
}
