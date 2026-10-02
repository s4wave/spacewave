package main

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
)

// TestChildExitCodePreservesProcessStatus checks that childExitCode returns the
// exit status of a child process.
func TestChildExitCodePreservesProcessStatus(t *testing.T) {
	// Run the helper as a child that exits with status 42.
	cmd := exec.Command(os.Args[0], "-test.run=TestAutobunExitHelper", "--")
	cmd.Env = append(os.Environ(), "AUTOBUN_EXIT_HELPER=42")
	err := cmd.Run()

	// The child's status survives in the returned exit code.
	exitCode, ok := childExitCode(err)
	if !ok {
		t.Fatalf("expected child exit error, got %v", err)
	}
	if exitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", exitCode)
	}
}

// TestAutobunExitHelper exits with the status in AUTOBUN_EXIT_HELPER when it
// runs as a child of TestChildExitCodePreservesProcessStatus.
func TestAutobunExitHelper(t *testing.T) {
	// Run only as a child that names an exit status.
	raw := os.Getenv("AUTOBUN_EXIT_HELPER")
	if raw == "" {
		return
	}

	// Exit with that status.
	code, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatal(err)
	}
	os.Exit(code)
}
