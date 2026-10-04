package loadedplugins

import "testing"

func TestStateTracksDemandedPluginReadiness(t *testing.T) {
	// Prepare empty plugin state before checking its readiness snapshots.
	var state State

	// Confirm readiness waits for initial desired-plugin reconciliation.
	pending, initialCh := state.HasPendingAndWaitCh()
	if !pending {
		t.Fatal("expected readiness to wait for initial reconciliation")
	}

	// Reconcile one demanded plugin and observe the readiness notification.
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

	// Mark the plugin running before its registration pass becomes terminal.
	state.SetPluginState("plugin-a", true, false)

	// Verify RPC connectivity alone does not expose the plugin as loaded.
	ids, _ := state.GetAndWaitCh()
	if len(ids) != 0 {
		t.Fatalf("RPC-connected plugin reported loaded before registration: %v", ids)
	}
	pending, _ = state.HasPendingAndWaitCh()
	if !pending {
		t.Fatal("running must not imply registration complete")
	}

	// Complete the plugin registration and confirm readiness wakes.
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

	// Verify the completed plugin ID list is returned without exposing its backing slice.
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
	// Reconcile an empty desired set and check readiness completes immediately.
	var state State
	state.Reconcile(nil)
	pending, _ := state.HasPendingAndWaitCh()
	if pending {
		t.Fatal("zero demanded plugins must complete readiness")
	}
}
