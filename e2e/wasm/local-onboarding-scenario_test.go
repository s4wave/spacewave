//go:build !skip_e2e && !js

package wasm

import "testing"

func TestLocalOnboardingScenario(t *testing.T) {
	sess := testHarness.NewSession(t)
	scenario := CreateLocalOnboardingScenario(t, testHarness, sess)

	if scenario.GetSessionIndex() == 0 {
		t.Fatal("expected non-zero session index")
	}
	if scenario.GetSpaceID() == "" {
		t.Fatal("expected non-empty space ID")
	}
}
