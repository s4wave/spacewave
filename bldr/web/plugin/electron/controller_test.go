package electron

import (
	"os/exec"
	"testing"
)

func TestShouldExitWithoutRestart(t *testing.T) {
	if !shouldExitWithoutRestart(nil, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected clean exit + exit policy to stop restart")
	}
	if shouldExitWithoutRestart(nil, QuitPolicy_QUIT_POLICY_RESTART) {
		t.Fatal("expected restart policy to keep restart behavior")
	}
	if shouldExitWithoutRestart(exec.ErrNotFound, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected non-zero process exit to keep restart behavior")
	}
}
