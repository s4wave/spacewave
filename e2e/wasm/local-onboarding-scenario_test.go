//go:build !skip_e2e && !js

package wasm

import "testing"

func TestLocalOnboardingScenario(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	scenario := CreateLocalOnboardingScenario(t, harness(t), sess)

	if scenario.GetSessionIndex() == 0 {
		t.Fatal("expected non-zero session index")
	}
	if scenario.GetSpaceID() == "" {
		t.Fatal("expected non-empty space ID")
	}
}
