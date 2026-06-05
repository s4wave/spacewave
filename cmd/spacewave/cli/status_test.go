//go:build !js

package spacewave_cli

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

func TestRunStatusMountTimeoutText(t *testing.T) {
	out, err := runStatusMountTimeout(t, "text")
	if err == nil {
		t.Fatal("expected status mount timeout error")
	}
	assertContains(t, err.Error(), "mount session timed out")
	assertContains(t, out, "Stage")
	assertContains(t, out, "mount session")
	assertContains(t, out, "Error")
	assertContains(t, out, "timed out")
}

func TestRunStatusMountTimeoutJSON(t *testing.T) {
	out, err := runStatusMountTimeout(t, "json")
	if err == nil {
		t.Fatal("expected status mount timeout error")
	}
	assertContains(t, err.Error(), "mount session timed out")
	assertContains(t, out, `"stage":"mount session"`)
	assertContains(t, out, `"error":"mount session timed out`)
}

func TestRunStatusMountTimeoutYAML(t *testing.T) {
	out, err := runStatusMountTimeout(t, "yaml")
	if err == nil {
		t.Fatal("expected status mount timeout error")
	}
	assertContains(t, err.Error(), "mount session timed out")
	assertContains(t, out, "stage: mount session")
	assertContains(t, out, "error: mount session timed out")
}

func TestGetStatusMountSessionTimeoutDefault(t *testing.T) {
	t.Setenv(statusMountSessionTimeoutEnvVar, "")

	dur, err := getStatusMountSessionTimeout()
	if err != nil {
		t.Fatal(err)
	}
	if dur != defaultStatusMountSessionTimeout {
		t.Fatalf("got %v, want %v", dur, defaultStatusMountSessionTimeout)
	}
}

func TestGetStatusMountSessionTimeoutOverrideDoesNotAffectDaemonStartup(t *testing.T) {
	t.Setenv(statusMountSessionTimeoutEnvVar, "25ms")
	t.Setenv(daemonStartupTimeoutEnvVar, "")

	statusTimeout, err := getStatusMountSessionTimeout()
	if err != nil {
		t.Fatal(err)
	}
	if statusTimeout != 25*time.Millisecond {
		t.Fatalf("status timeout = %v, want 25ms", statusTimeout)
	}

	startupTimeout, err := getDaemonStartupTimeout()
	if err != nil {
		t.Fatal(err)
	}
	if startupTimeout != defaultDaemonStartupTimeout {
		t.Fatalf("daemon startup timeout = %v, want %v", startupTimeout, defaultDaemonStartupTimeout)
	}
}

func TestGetStatusMountSessionTimeoutInvalid(t *testing.T) {
	t.Setenv(statusMountSessionTimeoutEnvVar, "not-a-duration")

	_, err := getStatusMountSessionTimeout()
	if err == nil {
		t.Fatal("expected invalid status mount timeout error")
	}
	assertContains(t, err.Error(), statusMountSessionTimeoutEnvVar)
}

func runStatusMountTimeout(t *testing.T, outputFormat string) (string, error) {
	t.Helper()

	restore := stubStatusTestHooks(t)
	defer restore()
	t.Setenv(statusMountSessionTimeoutEnvVar, "1ms")

	c := cli.NewContext(nil, emptyFlagSet(t), nil)
	c.Context = context.Background()

	return captureStdout(t, func() error {
		return runStatus(c, ".spacewave", outputFormat, 1)
	})
}

func stubStatusTestHooks(t *testing.T) func() {
	t.Helper()

	oldConnectDaemon := statusConnectDaemon
	oldCloseClient := statusCloseClient
	oldMountSession := statusMountSession
	oldResolveStatePath := statusResolveStatePath
	oldEffectiveSocketPath := statusEffectiveSocketPath

	statusConnectDaemon = func(ctx context.Context, c *cli.Context, statePath string) (*sdkClient, error) {
		if statePath != ".spacewave" {
			t.Fatalf("unexpected state path: %s", statePath)
		}
		return &sdkClient{}, nil
	}
	statusCloseClient = func(*sdkClient) {}
	statusMountSession = func(ctx context.Context, client *sdkClient, idx uint32) (*s4wave_session.Session, error) {
		if idx != 1 {
			t.Fatalf("unexpected session index: %d", idx)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	statusResolveStatePath = func(c *cli.Context, statePath string) (string, error) {
		t.Fatal("unexpected state-path resolution with socket-path hook")
		return "", nil
	}
	statusEffectiveSocketPath = func(c *cli.Context, fallback string) string {
		if fallback != "" {
			t.Fatalf("unexpected socket fallback: %s", fallback)
		}
		return "/tmp/spacewave.sock"
	}

	return func() {
		statusConnectDaemon = oldConnectDaemon
		statusCloseClient = oldCloseClient
		statusMountSession = oldMountSession
		statusResolveStatePath = oldResolveStatePath
		statusEffectiveSocketPath = oldEffectiveSocketPath
	}
}
