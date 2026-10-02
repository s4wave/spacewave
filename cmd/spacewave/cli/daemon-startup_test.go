//go:build !js

package spacewave_cli

import (
	"context"
	"flag"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/util/pipesock"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/s4wave/spacewave/core/daemon"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
)

func TestInvalidDaemonIdleTimeoutReportsStartupError(t *testing.T) {
	// Set an invalid idle timeout and open the startup pipe.
	t.Setenv(daemonIdleTimeoutEnvVar, "not-a-duration")
	statePath := shortSocketDir(t)
	pipeListener, err := pipesock.BuildPipeListener(daemon.NewStartupPipeLogger(), statePath, "startup")
	if err != nil {
		t.Fatal(err)
	}
	defer pipeListener.Close()

	// Build a serve context with the state path.
	app := cli.NewApp()
	parentFlags := flag.NewFlagSet("spacewave", flag.ContinueOnError)
	parentFlags.SetOutput(os.Stderr)
	parentFlags.String("state-path", statePath, "state directory path")
	if err := parentFlags.Parse([]string{"--state-path", statePath}); err != nil {
		t.Fatal(err)
	}
	parent := cli.NewContext(app, parentFlags, nil)
	child := cli.NewContext(app, flag.NewFlagSet("serve", flag.ContinueOnError), parent)

	// Run the serve command.
	commandErrCh := make(chan error, 1)
	go func() {
		commandErrCh <- runServeCommand(child, func() cli_entrypoint.CliBus { return nil }, yield_policy.NewBroker(), "startup", false, defaultDaemonIdleTimeout)
	}()

	// Require the startup error to name the idle-timeout variable.
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	startupErr := daemon.WaitStartup(waitCtx, pipeListener)
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
	t.Setenv(daemon.TracePathEnvVar, "")

	got := daemon.ServeArgs("/tmp/state", "pipe-id")
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
	t.Setenv(daemon.TracePathEnvVar, "/tmp/spacewave.trace")

	got := daemon.ServeArgs("/tmp/state", "pipe-id")
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
	t.Setenv(daemon.StartupTimeoutEnvVar, "")

	dur, err := daemon.StartupTimeout()
	if err != nil {
		t.Fatal(err)
	}
	if dur != daemon.DefaultStartupTimeout {
		t.Fatalf("got %v, want %v", dur, daemon.DefaultStartupTimeout)
	}
}

func TestGetDaemonStartupTimeoutOverride(t *testing.T) {
	t.Setenv(daemon.StartupTimeoutEnvVar, "75s")

	dur, err := daemon.StartupTimeout()
	if err != nil {
		t.Fatal(err)
	}
	if dur != 75*time.Second {
		t.Fatalf("got %v, want %v", dur, 75*time.Second)
	}
}

func TestGetDaemonStartupTimeoutInvalid(t *testing.T) {
	t.Setenv(daemon.StartupTimeoutEnvVar, "definitely-not-a-duration")

	_, err := daemon.StartupTimeout()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDaemonStartupPipeLoggerCanBuildListener(t *testing.T) {
	root := shortSocketDir(t)

	listener, err := pipesock.BuildPipeListener(daemon.NewStartupPipeLogger(), root, "startup")
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}
