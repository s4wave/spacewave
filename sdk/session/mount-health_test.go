package s4wave_session

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/core/sobject"
)

// mountHealthServer supplies one mount response through the real SRPC codec.
type mountHealthServer struct {
	SRPCSessionResourceServiceServer
	response *MountSharedObjectResponse
}

// MountSharedObject returns the fixture's typed result.
func (s *mountHealthServer) MountSharedObject(context.Context, *MountSharedObjectRequest) (*MountSharedObjectResponse, error) {
	return s.response, nil
}

// TestMountSharedObjectHealth preserves remediation through the Go SDK used by
// CLI callers, instead of letting a refusal appear to mount resource zero.
func TestMountSharedObjectHealth(t *testing.T) {
	health := sobject.NewSharedObjectClosedHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_ACCESS_REVOKED,
		sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REQUEST_ACCESS,
		"opaque diagnostic",
	)
	for _, response := range []*MountSharedObjectResponse{{Health: health}, {ResourceId: 7}} {
		mux := srpc.NewMux()
		if err := SRPCRegisterSessionResourceService(mux, &mountHealthServer{response: response}); err != nil {
			t.Fatal(err)
		}
		session := &Session{service: NewSRPCSessionResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))}
		got, err := session.MountSharedObject(t.Context(), "object")
		if response.GetHealth() != nil {
			actual, ok := sobject.GetSharedObjectHealthFromError(err)
			if got != nil || !ok || !actual.EqualVT(health) {
				t.Fatalf("lost mount health: response=%v error=%v", got, err)
			}
		} else if err != nil || got.GetResourceId() != 7 {
			t.Fatalf("successful mount changed: response=%v error=%v", got, err)
		}
	}
}
