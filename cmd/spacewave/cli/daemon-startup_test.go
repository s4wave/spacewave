//go:build !js

package spacewave_cli

import (
	"context"
	"flag"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/util/pipesock"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
)

// shortStatePath returns a state directory whose unix socket paths fit in
// sun_path. t.TempDir() derives its name from the test, and a long test name
// pushes the resulting socket path past the platform limit.
func shortStatePath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestInvalidDaemonIdleTimeoutReportsStartupError(t *testing.T) {
	t.Setenv(daemonIdleTimeoutEnvVar, "not-a-duration")
	statePath := shortStatePath(t)
	pipeListener, err := pipesock.BuildPipeListener(newDaemonStartupPipeLogger(), statePath, "startup")
	if err != nil {
		t.Fatal(err)
	}
	defer pipeListener.Close()

	app := cli.NewApp()
	parentFlags := flag.NewFlagSet("spacewave", flag.ContinueOnError)
	parentFlags.SetOutput(os.Stderr)
	parentFlags.String("state-path", statePath, "state directory path")
	if err := parentFlags.Parse([]string{"--state-path", statePath}); err != nil {
		t.Fatal(err)
	}
	parent := cli.NewContext(app, parentFlags, nil)
	child := cli.NewContext(app, flag.NewFlagSet("serve", flag.ContinueOnError), parent)

	commandErrCh := make(chan error, 1)
	go func() {
		commandErrCh <- runServeCommand(child, func() cli_entrypoint.CliBus { return nil }, yield_policy.NewBroker(), "startup", false, defaultDaemonIdleTimeout)
	}()

	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	startupErr := waitForDaemonStartup(waitCtx, pipeListener)
	commandErr := <-commandErrCh
	if startupErr == nil {
		t.Fatal("expected daemon startup error")
	}
	if !strings.Contains(startupErr.Error(), daemonIdleTimeoutEnvVar) {
		t.Fatalf("startup error = %v, command error = %v, want %q", startupErr, commandErr, daemonIdleTimeoutEnvVar)
	}
	if commandErr == nil {
		t.Fatal("expected serve command error")
	}
}

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

func TestDaemonStartupPipeLoggerCanBuildListener(t *testing.T) {
	if err := os.MkdirAll(".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(".tmp", "startup-pipe-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	listener, err := pipesock.BuildPipeListener(newDaemonStartupPipeLogger(), root, "startup")
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
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
