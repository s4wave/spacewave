//go:build !js

package spacewave_cli

import (
	"slices"
	"sort"
	"testing"
	"time"
)

func TestDaemonServeArgsPassStatePathToServe(t *testing.T) {
	t.Setenv(daemonTracePathEnvVar, "")

	got := daemonServeArgs("/tmp/state", "pipe-id")
	want := []string{
		"--state-path", "/tmp/state",
		"serve",
		"--daemon-startup-pipe-id", "pipe-id",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestDaemonServeArgsPassTracePathToServe(t *testing.T) {
	t.Setenv(daemonTracePathEnvVar, "/tmp/spacewave.trace")

	got := daemonServeArgs("/tmp/state", "pipe-id")
	want := []string{
		"--state-path", "/tmp/state",
		"serve",
		"--daemon-startup-pipe-id", "pipe-id",
		"--trace", "/tmp/spacewave.trace",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestGetDaemonStartupTimeoutDefault(t *testing.T) {
	t.Setenv(daemonStartupTimeoutEnvVar, "")

	dur, err := getDaemonStartupTimeout()
	if err != nil {
		t.Fatal(err)
	}
	if dur != defaultDaemonStartupTimeout {
		t.Fatalf("got %v, want %v", dur, defaultDaemonStartupTimeout)
	}
}

func TestGetDaemonStartupTimeoutOverride(t *testing.T) {
	t.Setenv(daemonStartupTimeoutEnvVar, "75s")

	dur, err := getDaemonStartupTimeout()
	if err != nil {
		t.Fatal(err)
	}
	if dur != 75*time.Second {
		t.Fatalf("got %v, want %v", dur, 75*time.Second)
	}
}

func TestGetDaemonStartupTimeoutInvalid(t *testing.T) {
	t.Setenv(daemonStartupTimeoutEnvVar, "definitely-not-a-duration")

	_, err := getDaemonStartupTimeout()
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestDaemonChildEnvForwardedContract pins the env-var contract for
// IC-4: any change to the daemon spawn path that sets cmd.Env
// explicitly must continue to forward this exact list. The list is
// kept sorted so additions are obvious in code review.
func TestDaemonChildEnvForwardedContract(t *testing.T) {
	want := []string{
		"BLDR_LOG_FILE",
		"BLDR_LOG_LEVEL",
		"BLDR_STATE_PATH",
		"SPACEWAVE_DAEMON_TRACE",
		"SPACEWAVE_DATA_DIR",
		"SPACEWAVE_LOG_LEVEL",
		"SPACEWAVE_LOG_RETENTION_DAYS",
	}
	got := slices.Clone(daemonChildEnvForwarded)
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Fatalf("daemonChildEnvForwarded =\n  got  %#v\n  want %#v", got, want)
	}
}
