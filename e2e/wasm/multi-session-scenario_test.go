//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"testing"
)

func TestMultiSessionScenario(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	scenario := CreateMultiSessionScenario(t, harness(t), sess)

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

	scenario.SwitchToSession(t, scenario.GetFirstSessionIndex())
	scenario.WaitForLocalBadge(t)
	scenario.LockFirstSessionAtNestedRoute(t)
	scenario.UnlockVisiblePIN(t)

	scenario.ExitToSessionSelector(t)
	scenario.SwitchToSession(t, scenario.GetSecondSessionIndex())
	scenario.WaitForLocalBadge(t)
}

func TestWaitForCountConsumesOwnerUpdates(t *testing.T) {
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
