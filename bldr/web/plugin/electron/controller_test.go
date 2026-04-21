package electron

import (
	"io"
	"os/exec"
	"testing"
)

func TestShouldExitWithoutRestart(t *testing.T) {
	if !shouldExitWithoutRestart(io.EOF, nil, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected EOF + clean exit + exit policy to stop restart")
	}
	if shouldExitWithoutRestart(io.EOF, nil, QuitPolicy_QUIT_POLICY_RESTART) {
		t.Fatal("expected restart policy to keep restart behavior")
	}
	if shouldExitWithoutRestart(exec.ErrNotFound, nil, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected non-EOF runtime error to keep restart behavior")
	}
	if shouldExitWithoutRestart(io.EOF, exec.ErrNotFound, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected non-zero process exit to keep restart behavior")
	}
}
