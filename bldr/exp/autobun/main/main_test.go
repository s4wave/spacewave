package main

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
)

func TestChildExitCodePreservesProcessStatus(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestAutobunExitHelper", "--")
	cmd.Env = append(os.Environ(), "AUTOBUN_EXIT_HELPER=42")
	err := cmd.Run()

	exitCode, ok := childExitCode(err)
	if !ok {
		t.Fatalf("expected child exit error, got %v", err)
	}
	if exitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", exitCode)
	}
}

func TestAutobunExitHelper(t *testing.T) {
	raw := os.Getenv("AUTOBUN_EXIT_HELPER")
	if raw == "" {
		return
	}
	code, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatal(err)
	}
	os.Exit(code)
}
