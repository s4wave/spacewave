package forge_runtime

import (
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// TestReservationUnmarshalJSONClearsReusedReceiver verifies that decoding
// into a reused Reservation leaves omitted optional fields unset.
func TestReservationUnmarshalJSONClearsReusedReceiver(t *testing.T) {
	// Start from a reservation holding every optional field.
	res := &Reservation{
		WorkerObjectKey: "workers/a",
		LeaseExpiresAt:  &timestamp.Timestamp{Seconds: 1},
		Runtime:         BackendRuntimeIdentity{ID: "runtime-a", StopEnv: []string{"A=1"}},
		Cleanup:         &CleanupReceipt{Reason: "stopped"},
	}

	// Decode a record that omits the optional fields.
	if err := res.UnmarshalJSON([]byte(`{"workerObjectKey":"workers/b"}`)); err != nil {
		t.Fatal(err)
	}

	// Expect only the decoded identity to remain.
	if res.WorkerObjectKey != "workers/b" {
		t.Fatalf("worker object key = %q", res.WorkerObjectKey)
	}
	if res.LeaseExpiresAt != nil || res.Runtime.ID != "" || res.Runtime.StopEnv != nil || res.Cleanup != nil {
		t.Fatalf("optional fields retained: %+v", res)
	}
}
