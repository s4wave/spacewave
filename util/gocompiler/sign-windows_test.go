package gocompiler

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestSignWindowsNoOpWhenUnset verifies that SignWindows returns nil without
// invoking az when BLDR_WINDOWS_SIGN_PROFILE is unset.
func TestSignWindowsNoOpWhenUnset(t *testing.T) {
	t.Setenv(WindowsSignProfileEnv, "")
	le := logrus.NewEntry(logrus.New())
	if err := SignWindows(context.Background(), le, "/nonexistent/path.exe"); err != nil {
		t.Fatalf("expected no-op with unset env, got error: %v", err)
	}
}

// TestSignWindowsRejectsProfileWithoutAccount verifies SignWindows errors when
// profile is set but account is missing.
func TestSignWindowsRejectsProfileWithoutAccount(t *testing.T) {
	t.Setenv(WindowsSignProfileEnv, "some-profile")
	t.Setenv(WindowsSignAccountEnv, "")
	le := logrus.NewEntry(logrus.New())
	if err := SignWindows(context.Background(), le, "/nonexistent/path.exe"); err == nil {
		t.Fatal("expected error when profile is set without account")
	}
}
