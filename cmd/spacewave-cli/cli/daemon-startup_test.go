package spacewave_cli

import (
	"slices"
	"testing"
	"time"
)

func TestDaemonServeArgsPassStatePathToServe(t *testing.T) {
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
