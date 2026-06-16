//go:build !skip_e2e && !js

package wasm

import "testing"

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
