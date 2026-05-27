package provider_spacewave

import (
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
)

func mustMarshalVT(t *testing.T, marshaler interface{ MarshalVT() ([]byte, error) }) []byte {
	t.Helper()
	data, err := marshaler.MarshalVT()
	if err != nil {
		t.Fatalf("marshal protobuf: %v", err)
	}
	return data
}

func mustMarshalSOStateMessageSnapshotJSON(t *testing.T, state *sobject.SOState) []byte {
	t.Helper()
	return mustMarshalVT(t, &api.SOStateMessage{
		Seqno:   1,
		Content: &api.SOStateMessage_Snapshot{Snapshot: state},
	})
}
