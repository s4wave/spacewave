package loadedplugins

import "testing"

func TestStateTracksDemandedPluginReadiness(t *testing.T) {
	var state State

	pending, initialCh := state.HasPendingAndWaitCh()
	if !pending {
		t.Fatal("expected readiness to wait for initial reconciliation")
	}

	state.Reconcile([]string{"plugin-a"})
	select {
	case <-initialCh:
	default:
		t.Fatal("expected reconciliation to wake readiness")
	}
	pending, readyCh := state.HasPendingAndWaitCh()
	if !pending {
		t.Fatal("expected demanded plugin registration to be pending")
	}

	state.SetPluginState("plugin-a", true, false)
	ids, _ := state.GetAndWaitCh()
	if len(ids) != 0 {
		t.Fatalf("RPC-connected plugin reported loaded before registration: %v", ids)
	}
	pending, _ = state.HasPendingAndWaitCh()
	if !pending {
		t.Fatal("running must not imply registration complete")
	}

	state.SetPluginState("plugin-a", true, true)
	select {
	case <-readyCh:
	default:
		t.Fatal("expected terminal registration to wake readiness")
	}
	pending, _ = state.HasPendingAndWaitCh()
	if pending {
		t.Fatal("expected all demanded plugin registrations to be terminal")
	}

	ids, _ = state.GetAndWaitCh()
	if len(ids) != 1 || ids[0] != "plugin-a" {
		t.Fatalf("unexpected loaded plugin ids: %v", ids)
	}
	ids[0] = "mutated"
	next, _ := state.GetAndWaitCh()
	if len(next) != 1 || next[0] != "plugin-a" {
		t.Fatalf("running plugin ids alias leaked: %v", next)
	}
}

func TestStateUnknownTypeCanFinishWithZeroDemandedPlugins(t *testing.T) {
	var state State
	state.Reconcile(nil)
	pending, _ := state.HasPendingAndWaitCh()
	if pending {
		t.Fatal("zero demanded plugins must complete readiness")
	}
}
