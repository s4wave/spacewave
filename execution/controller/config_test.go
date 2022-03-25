package execution_controller

import "testing"

// TestUniqueID tests the deterministic unique id for consistency.
func TestUniqueID(t *testing.T) {
	c := &Config{
		PeerId:    "test-peer-id",
		ObjectKey: "test-object-key",
	}
	uuid := c.BuildUniqueID()
	expected := "07ed81a0-ae20-2130-4da0-8a055a4a463f"
	if uuid != expected {
		t.Fatalf("expected %s but got %s", expected, uuid)
	}
}
