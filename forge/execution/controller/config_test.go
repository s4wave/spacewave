package execution_controller

import "testing"

// TestUniqueID tests the deterministic unique id for consistency.
func TestUniqueID(t *testing.T) {
	c := &Config{
		EngineId:  "test-engine-id",
		PeerId:    "test-peer-id",
		ObjectKey: "test-object-key",
	}
	id := c.BuildUniqueID()
	expected := "7bd54ab7-473a-58c2-fd94-6ab07d04857d"
	if id != expected {
		t.Fatalf("expected %s but got %s", expected, id)
	}
}

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
