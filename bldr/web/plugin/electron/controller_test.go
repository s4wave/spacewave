package electron

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestShouldExitWithoutRestart(t *testing.T) {
	if !shouldExitWithoutRestart(errors.New("stream reset"), nil, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected clean exit + exit policy to stop restart")
	}
	if shouldExitWithoutRestart(errors.New("stream reset"), nil, QuitPolicy_QUIT_POLICY_RESTART) {
		t.Fatal("expected restart policy to keep restart behavior")
	}
	if shouldExitWithoutRestart(nil, exec.ErrNotFound, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected non-zero process exit to keep restart behavior")
	}
	if !shouldExitWithoutRestart(errors.New("stream reset"), context.DeadlineExceeded, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected stream reset + exit policy to stop restart")
	}
	if shouldExitWithoutRestart(errors.New("unexpected disconnect"), context.DeadlineExceeded, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected unexpected disconnect to keep restart behavior")
	}
}
