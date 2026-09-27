package s4wave_sobject

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/core/sobject"
)

// mountHealthServer supplies a body mount response through the SRPC codec.
type mountHealthServer struct {
	SRPCSharedObjectResourceServiceServer
	response *MountSharedObjectBodyResponse
}

// MountSharedObjectBody returns the fixture's typed result.
func (s *mountHealthServer) MountSharedObjectBody(context.Context, *MountSharedObjectBodyRequest) (*MountSharedObjectBodyResponse, error) {
	return s.response, nil
}

// TestMountSharedObjectBodyHealth preserves health through the Go SDK boundary.
func TestMountSharedObjectBodyHealth(t *testing.T) {
	health := sobject.NewSharedObjectClosedHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_ACCESS_REVOKED,
		sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REQUEST_ACCESS,
		"opaque diagnostic",
	)
	for _, response := range []*MountSharedObjectBodyResponse{
		{Result: &MountSharedObjectBodyResponse_Health{Health: health}},
		{Result: &MountSharedObjectBodyResponse_ResourceId{ResourceId: 7}},
	} {
		mux := srpc.NewMux()
		if err := SRPCRegisterSharedObjectResourceService(mux, &mountHealthServer{response: response}); err != nil {
			t.Fatal(err)
		}
		client := NewSRPCSharedObjectResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
		got, err := MountSharedObjectBody(t.Context(), client)
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
