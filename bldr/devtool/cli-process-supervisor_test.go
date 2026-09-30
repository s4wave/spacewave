//go:build !js

package devtool

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestCliSubprocessSupervisorWaitReturnsExitError(t *testing.T) {
	// Start a helper that exits with code 7 under the supervisor.
	supervisor := newCliSubprocessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	cmd := cliSubprocessSupervisorTestCommand(t, "exit", "7")
	if err := supervisor.startCommand(cmd); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	// Expect the wait channel to report the exit code.
	err := <-supervisor.wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("wait error = %T %[1]v, want exec.ExitError", err)
	}
	if exitErr.ExitCode() != 7 {
		t.Fatalf("exit code = %d, want 7", exitErr.ExitCode())
	}
}

func TestCliSubprocessSupervisorStartContextCancelUsesTerminatePath(t *testing.T) {
	// Start a helper that waits on SIGTERM under a cancellable context.
	t.Setenv("BLDR_DEVTOOL_CLI_PROCESS_HELPER", "1")

	// Start the term-exit helper under a cancellable supervisor context.
	ctx, cancel := context.WithCancel(context.Background())
	readyPath := filepath.Join(t.TempDir(), "ready")
	supervisor := newCliSubprocessSupervisor(
		ctx,
		logrus.NewEntry(logrus.New()),
		cliSubprocessSupervisorTestExecutable(t),
		cliSubprocessSupervisorTestArgs("term-exit", readyPath),
	)
	if err := supervisor.start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitForCliSubprocessHelperReady(t, readyPath)

	// Cancel the context and expect the process to survive until termination.
	cancel()
	select {
	case err := <-supervisor.wait():
		t.Fatalf("context cancel ended process before supervisor termination: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	// Terminate the surviving helper process.
	if err := supervisor.terminateAfter(5 * time.Second); err != nil {
		t.Fatalf("terminate after context cancel: %v", err)
	}
}

func TestCliSubprocessSupervisorClosesStderrAfterWait(t *testing.T) {
	// Run a helper to completion with a recording stderr writer.
	supervisor := newCliSubprocessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	stderr := newCliSubprocessSupervisorCloseRecorder()
	supervisor.stderr = stderr
	cmd := cliSubprocessSupervisorTestCommand(t, "exit", "0")
	if err := supervisor.startCommand(cmd); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	if err := <-supervisor.wait(); err != nil {
		t.Fatalf("wait helper: %v", err)
	}

	// Expect the stderr writer to be closed after the wait.
	stderr.expectClosed(t)
}

func TestCliSubprocessSupervisorClosesStderrAfterStartError(t *testing.T) {
	// Attempt to start a missing helper with a recording stderr writer.
	supervisor := newCliSubprocessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	stderr := newCliSubprocessSupervisorCloseRecorder()
	supervisor.stderr = stderr
	err := supervisor.startCommand(exec.Command(filepath.Join(t.TempDir(), "missing"))) //nolint:gosec // G204: test path is a temp dir under test control
	if err == nil {
		t.Fatal("start missing helper returned nil, want error")
	}

	// Expect the stderr writer to be closed after the start error.
	stderr.expectClosed(t)
}

func TestCliSubprocessSupervisorTerminateWaitsForSignalExit(t *testing.T) {
	// Start a helper that exits on SIGTERM.
	supervisor := newCliSubprocessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	readyPath := filepath.Join(t.TempDir(), "ready")
	cmd := cliSubprocessSupervisorTestCommand(t, "term-exit", readyPath)
	if err := supervisor.startCommand(cmd); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitForCliSubprocessHelperReady(t, readyPath)

	// Expect termination to complete through the signal exit.
	if err := supervisor.terminateAfter(5 * time.Second); err != nil {
		t.Fatalf("terminate: %v", err)
	}
}

func TestCliSubprocessSupervisorTerminateKillsAfterTimeout(t *testing.T) {
	// Start a helper that ignores SIGTERM.
	supervisor := newCliSubprocessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	readyPath := filepath.Join(t.TempDir(), "ready")
	cmd := cliSubprocessSupervisorTestCommand(t, "ignore-term", readyPath)
	if err := supervisor.startCommand(cmd); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitForCliSubprocessHelperReady(t, readyPath)

	// Expect termination to kill the process within a bounded timeout.
	started := time.Now()
	err := supervisor.terminateAfter(50 * time.Millisecond)
	if err == nil {
		t.Fatal("terminate returned nil, want killed process error")
	}
	if time.Since(started) > time.Second {
		t.Fatalf("terminate took %s, want bounded kill timeout", time.Since(started))
	}
	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Fatalf("terminate error = %T %[1]v, want exec.ExitError", err)
	}
}

func TestCliSubprocessSupervisorHelper(t *testing.T) {
	// Run only as the re-executed helper process.
	if os.Getenv("BLDR_DEVTOOL_CLI_PROCESS_HELPER") != "1" {
		return
	}

	// Dispatch on the requested helper mode.
	args := cliSubprocessSupervisorHelperArgs()
	if len(args) == 0 {
		os.Exit(2)
	}

	switch args[0] {
	case "exit":
		code, err := strconv.Atoi(args[1])
		if err != nil {
			os.Exit(2)
		}
		os.Exit(code)
	case "term-exit":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		markCliSubprocessHelperReady(args[1])
		<-signals
		os.Exit(0)
	case "ignore-term":
		signal.Ignore(syscall.SIGTERM)
		markCliSubprocessHelperReady(args[1])
		select {}
	default:
		os.Exit(2)
	}
}

func cliSubprocessSupervisorTestCommand(t *testing.T, mode string, args ...string) *exec.Cmd {
	// Attribute test failures to the caller.
	t.Helper()

	// Build a re-executed test command running the helper entry point.
	cmdArgs := append([]string{"-test.run=TestCliSubprocessSupervisorHelper", "--", mode}, args...)
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	cmd := exec.Command(executable, cmdArgs...)
	cmd.Env = append(os.Environ(), "BLDR_DEVTOOL_CLI_PROCESS_HELPER=1")
	return cmd
}

func cliSubprocessSupervisorTestExecutable(t *testing.T) string {
	// Resolve this test binary's path for re-execution.
	t.Helper()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	return executable
}

func cliSubprocessSupervisorTestArgs(mode string, args ...string) []string {
	// Build the helper re-execution arguments for the given mode.
	return append([]string{"-test.run=TestCliSubprocessSupervisorHelper", "--", mode}, args...)
}

func cliSubprocessSupervisorHelperArgs() []string {
	// Return the arguments after the -- separator.
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	return nil
}

func markCliSubprocessHelperReady(path string) {
	// Write the ready marker file at the given path.
	if err := os.WriteFile(path, []byte("ready"), 0o644); err != nil {
		os.Exit(2)
	}
}

type cliSubprocessSupervisorCloseRecorder struct {
	once   sync.Once
	closed chan struct{}
}

func newCliSubprocessSupervisorCloseRecorder() *cliSubprocessSupervisorCloseRecorder {
	return &cliSubprocessSupervisorCloseRecorder{closed: make(chan struct{})}
}

func (r *cliSubprocessSupervisorCloseRecorder) Close() error {
	r.once.Do(func() {
		close(r.closed)
	})
	return nil
}

func (r *cliSubprocessSupervisorCloseRecorder) expectClosed(t *testing.T) {
	t.Helper()

	select {
	case <-r.closed:
	case <-time.After(time.Second):
		t.Fatal("stderr writer was not closed")
	}
}

func waitForCliSubprocessHelperReady(t *testing.T, path string) {
	// Poll for the ready marker until the deadline.
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("helper did not become ready at %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
