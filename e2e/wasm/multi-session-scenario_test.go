//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"testing"
)

func TestMultiSessionScenario(t *testing.T) {
	// Create a two-session local scenario.
	sess := harness(t).NewCleanSession(t)
	scenario := CreateMultiSessionScenario(t, harness(t), sess)

	// Require two distinct session indexes and a first Space.
	if scenario.GetFirstSessionIndex() == 0 {
		t.Fatal("expected non-zero first session index")
	}
	if scenario.GetSecondSessionIndex() == 0 {
		t.Fatal("expected non-zero second session index")
	}
	if scenario.GetFirstSessionIndex() == scenario.GetSecondSessionIndex() {
		t.Fatalf("expected distinct sessions, got %d", scenario.GetFirstSessionIndex())
	}
	if scenario.GetFirstSpaceID() == "" {
		t.Fatal("expected first session drive space")
	}

	// Switch to the first session, lock it, and unlock the PIN.
	scenario.SwitchToSession(t, scenario.GetFirstSessionIndex())
	scenario.WaitForLocalBadge(t)
	scenario.LockFirstSessionAtNestedRoute(t)
	scenario.UnlockVisiblePIN(t)

	// Switch to the second session and require its local badge.
	scenario.ExitToSessionSelector(t)
	scenario.SwitchToSession(t, scenario.GetSecondSessionIndex())
	scenario.WaitForLocalBadge(t)
}

func TestWaitForCountConsumesOwnerUpdates(t *testing.T) {
	// Consume successive owner counts until the target is reached.
	updates := []int{1, 2}
	calls := 0
	err := waitForCount(context.Background(), func(context.Context) (int, error) {
		calls++
		count := updates[0]
		updates = updates[1:]
		return count, nil
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("wait calls = %d, want 2", calls)
	}
}
