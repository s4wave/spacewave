//go:build !js

package spacewave_cli

import (
	"testing"
	"time"
)

func TestGetStatusMountSessionTimeoutDefault(t *testing.T) {
	t.Setenv(statusMountSessionTimeoutEnvVar, "")
	oldDefault := defaultStatusMountSessionTimeout
	defaultStatusMountSessionTimeout = 37 * time.Millisecond
	defer func() { defaultStatusMountSessionTimeout = oldDefault }()

	got, err := getStatusMountSessionTimeout()
	if err != nil {
		t.Fatalf("get timeout: %v", err)
	}
	if got != 37*time.Millisecond {
		t.Fatalf("timeout = %s", got)
	}
}

func TestGetStatusMountSessionTimeoutOverrideDoesNotAffectDaemonStartup(t *testing.T) {
	t.Setenv(statusMountSessionTimeoutEnvVar, "2ms")
	got, err := getStatusMountSessionTimeout()
	if err != nil {
		t.Fatalf("get timeout: %v", err)
	}
	if got != 2*time.Millisecond {
		t.Fatalf("timeout = %s", got)
	}
}

func TestGetStatusMountSessionTimeoutInvalid(t *testing.T) {
	t.Setenv(statusMountSessionTimeoutEnvVar, "nope")
	if _, err := getStatusMountSessionTimeout(); err == nil {
		t.Fatal("expected invalid timeout error")
	}
}
