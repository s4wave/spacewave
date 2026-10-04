package execution_controller

import "testing"

// TestUniqueID tests the deterministic unique id for consistency.
func TestUniqueID(t *testing.T) {
	// Pin the deterministic identity used when reconstructing this controller.
	c := &Config{
		EngineId:  "test-engine-id",
		PeerId:    "test-peer-id",
		ObjectKey: "test-object-key",
	}
	id := c.BuildUniqueID()
	expected := "2968eb94-73d3-9cb9-1411-09e7955cda40"
	if id != expected {
		t.Fatalf("expected %s but got %s", expected, id)
	}

	// Keep distinct Workers on the same peer from sharing a claim identity.
	first, second := c.CloneVT(), c.CloneVT()
	first.WorkerObjectKey = "workers/first"
	second.WorkerObjectKey = "workers/second"
	if first.BuildUniqueID() == second.BuildUniqueID() || first.BuildUniqueID() == id {
		t.Fatal("Worker placement did not distinguish controller identities")
	}
}

// TestNewConfigBuildsDeterministicClaimID preserves ownership on reconstruction.
func TestNewConfigBuildsDeterministicClaimID(t *testing.T) {
	first := NewConfig("test-engine-id", "test-object-key", "", nil)
	second := NewConfig("test-engine-id", "test-object-key", "", nil)
	if first.GetClaimId() != first.BuildUniqueID() {
		t.Fatalf("claim ID %q does not match durable owner ID %q", first.GetClaimId(), first.BuildUniqueID())
	}
	if second.GetClaimId() != first.GetClaimId() {
		t.Fatalf("reconstructed claim ID %q does not match %q", second.GetClaimId(), first.GetClaimId())
	}
}
