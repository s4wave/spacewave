package gocompiler

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestSignWindowsNoOpWhenUnset verifies that SignWindows returns nil without
// invoking pwsh when BLDR_WINDOWS_SIGN_PROFILE is unset.
func TestSignWindowsNoOpWhenUnset(t *testing.T) {
	t.Setenv(WindowsSignProfileEnv, "")
	t.Setenv(WindowsSignCommandEnv, "")
	le := logrus.NewEntry(logrus.New())
	if err := SignWindows(context.Background(), le, "/nonexistent/path.exe"); err != nil {
		t.Fatalf("expected no-op with unset env, got error: %v", err)
	}
}

// TestSignWindowsRejectsProfileWithoutAccount verifies SignWindows errors when
// profile is set but account is missing.
func TestSignWindowsRejectsProfileWithoutAccount(t *testing.T) {
	t.Setenv(WindowsSignCommandEnv, "")
	t.Setenv(WindowsSignProfileEnv, "some-profile")
	t.Setenv(WindowsSignAccountEnv, "")
	le := logrus.NewEntry(logrus.New())
	if err := SignWindows(context.Background(), le, "/nonexistent/path.exe"); err == nil {
		t.Fatal("expected error when profile is set without account")
	}
}

// TestSignWindowsExternalCommand checks path handling and failed signing propagation.
func TestSignWindowsExternalCommand(t *testing.T) {
	// The remote-signing cross-build uses a POSIX executable without shell expansion.
	if runtime.GOOS == "windows" {
		t.Skip("POSIX callback fixture")
	}
	dir := t.TempDir()
	command := filepath.Join(dir, "sign")
	binary := filepath.Join(dir, "app with spaces; literal.exe")
	if err := os.WriteFile(command, []byte("#!/bin/sh\n[ \"$#\" -eq 1 ] || exit 2\nprintf signed > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(WindowsSignCommandEnv, command)
	t.Setenv(WindowsSignIdentityEnv, "example-product/publisher")
	t.Setenv(WindowsSignProfileEnv, "")
	le := logrus.NewEntry(logrus.New())

	// The callback receives the complete path as one argument and finishes before return.
	if err := SignWindows(t.Context(), le, binary); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "signed" {
		t.Fatalf("unexpected signed contents: %q", data)
	}

	// A failed signing operation cannot be treated as a completed build.
	if err := os.WriteFile(command, []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SignWindows(t.Context(), le, binary); err == nil {
		t.Fatal("failed signing command returned success")
	}
}
