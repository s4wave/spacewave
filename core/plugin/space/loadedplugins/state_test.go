package loadedplugins

import "testing"

func TestStateSnapshotAndWake(t *testing.T) {
	var state State

	state.Set([]string{"plugin-a"})
	ids, ch := state.GetAndWaitCh()
	if len(ids) != 1 || ids[0] != "plugin-a" {
		t.Fatalf("unexpected loaded plugin ids: %v", ids)
	}

	ids[0] = "mutated"
	next, _ := state.GetAndWaitCh()
	if len(next) != 1 || next[0] != "plugin-a" {
		t.Fatalf("loaded plugin ids alias leaked: %v", next)
	}

	state.Set([]string{"plugin-a", "plugin-b"})
	select {
	case <-ch:
	default:
		t.Fatal("expected wait channel to close after loaded plugin set changed")
	}
}
