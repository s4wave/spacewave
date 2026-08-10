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
	supervisor := NewCLIProcessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	cmd := cliSubprocessSupervisorTestCommand(t, "exit", "7")
	if err := supervisor.startCommand(cmd); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	err := supervisor.Wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("wait error = %T %[1]v, want exec.ExitError", err)
	}
	if exitErr.ExitCode() != 7 {
		t.Fatalf("exit code = %d, want 7", exitErr.ExitCode())
	}
}

func TestCliSubprocessSupervisorStartContextCancelUsesTerminatePath(t *testing.T) {
	t.Setenv("BLDR_DEVTOOL_CLI_PROCESS_HELPER", "1")

	ctx, cancel := context.WithCancel(context.Background())
	readyPath := filepath.Join(t.TempDir(), "ready")
	supervisor := NewCLIProcessSupervisor(
		ctx,
		logrus.NewEntry(logrus.New()),
		cliSubprocessSupervisorTestExecutable(t),
		cliSubprocessSupervisorTestArgs("term-exit", readyPath),
	)
	if err := supervisor.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitForCliSubprocessHelperReady(t, readyPath)

	cancel()
	select {
	case <-supervisor.Done():
		t.Fatalf("context cancel ended process before supervisor termination: %v", supervisor.Wait())
	case <-time.After(100 * time.Millisecond):
	}

	if err := supervisor.terminateAfter(5 * time.Second); err != nil {
		t.Fatalf("terminate after context cancel: %v", err)
	}
}

func TestCliSubprocessSupervisorClosesStderrAfterWait(t *testing.T) {
	supervisor := NewCLIProcessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	stderr := newCLIProcessSupervisorCloseRecorder()
	supervisor.stderr = stderr
	cmd := cliSubprocessSupervisorTestCommand(t, "exit", "0")
	if err := supervisor.startCommand(cmd); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	if err := supervisor.Wait(); err != nil {
		t.Fatalf("wait helper: %v", err)
	}
	stderr.expectClosed(t)
}

func TestCliSubprocessSupervisorClosesStderrAfterStartError(t *testing.T) {
	supervisor := NewCLIProcessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	stderr := newCLIProcessSupervisorCloseRecorder()
	supervisor.stderr = stderr
	err := supervisor.startCommand(exec.Command(filepath.Join(t.TempDir(), "missing"))) //nolint:gosec // G204: test path is a temp dir under test control
	if err == nil {
		t.Fatal("start missing helper returned nil, want error")
	}
	stderr.expectClosed(t)
}

func TestCliSubprocessSupervisorTerminateWaitsForSignalExit(t *testing.T) {
	supervisor := NewCLIProcessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	readyPath := filepath.Join(t.TempDir(), "ready")
	cmd := cliSubprocessSupervisorTestCommand(t, "term-exit", readyPath)
	if err := supervisor.startCommand(cmd); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitForCliSubprocessHelperReady(t, readyPath)

	if err := supervisor.terminateAfter(5 * time.Second); err != nil {
		t.Fatalf("terminate: %v", err)
	}
}

func TestCliSubprocessSupervisorTerminateKillsAfterTimeout(t *testing.T) {
	supervisor := NewCLIProcessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	readyPath := filepath.Join(t.TempDir(), "ready")
	cmd := cliSubprocessSupervisorTestCommand(t, "ignore-term", readyPath)
	if err := supervisor.startCommand(cmd); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitForCliSubprocessHelperReady(t, readyPath)

	started := time.Now()
	err := supervisor.terminateAfter(50 * time.Millisecond)
	if err == nil {
		t.Fatal("terminate returned nil, want killed process error")
	}
	if time.Since(started) > time.Second {
		t.Fatalf("terminate took %s, want bounded kill timeout", time.Since(started))
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("terminate error = %T %[1]v, want exec.ExitError", err)
	}
	if waitErr := supervisor.Wait(); waitErr != err {
		t.Fatalf("Wait error = %v, want stable kill error %v", waitErr, err)
	}
	if terminateErr := supervisor.Terminate(); terminateErr != err {
		t.Fatalf("repeated Terminate error = %v, want stable kill error %v", terminateErr, err)
	}
}

func TestCLIProcessSupervisorRejectsConcurrentStart(t *testing.T) {
	t.Setenv("BLDR_DEVTOOL_CLI_PROCESS_HELPER", "1")

	readyPath := filepath.Join(t.TempDir(), "ready")
	supervisor := NewCLIProcessSupervisor(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		cliSubprocessSupervisorTestExecutable(t),
		cliSubprocessSupervisorTestArgs("term-exit", readyPath),
	)

	const callers = 16
	results := make(chan error, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	start := make(chan struct{})
	for range callers {
		go func() {
			ready.Done()
			<-start
			results <- supervisor.Start()
		}()
	}
	ready.Wait()
	close(start)

	started := 0
	for range callers {
		err := <-results
		switch {
		case err == nil:
			started++
		case errors.Is(err, ErrCLIProcessAlreadyStarted):
		default:
			t.Fatalf("Start error = %v, want nil or ErrCLIProcessAlreadyStarted", err)
		}
	}
	if started != 1 {
		t.Fatalf("successful starts = %d, want 1", started)
	}
	waitForCliSubprocessHelperReady(t, readyPath)
	if err := supervisor.Terminate(); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	if err := supervisor.Start(); !errors.Is(err, ErrCLIProcessAlreadyStarted) {
		t.Fatalf("repeat Start error = %v, want ErrCLIProcessAlreadyStarted", err)
	}
}

func TestCLIProcessSupervisorWaitThenTerminateReturnsNaturalExit(t *testing.T) {
	supervisor := NewCLIProcessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	if err := supervisor.startCommand(cliSubprocessSupervisorTestCommand(t, "exit", "7")); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	waitErr := supervisor.Wait()
	terminateErr := supervisor.Terminate()
	var waitExitErr, terminateExitErr *exec.ExitError
	if !errors.As(waitErr, &waitExitErr) || !errors.As(terminateErr, &terminateExitErr) {
		t.Fatalf("results = (%T %v, %T %v), want exit errors", waitErr, waitErr, terminateErr, terminateErr)
	}
	if waitExitErr != terminateExitErr || waitExitErr.ExitCode() != 7 {
		t.Fatalf("results are not the stable exit error: (%p, %p)", waitExitErr, terminateExitErr)
	}
}

func TestCLIProcessSupervisorBroadcastsResultToWaitAndTerminate(t *testing.T) {
	supervisor := NewCLIProcessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	readyPath := filepath.Join(t.TempDir(), "ready")
	if err := supervisor.startCommand(cliSubprocessSupervisorTestCommand(t, "term-exit", readyPath)); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitForCliSubprocessHelperReady(t, readyPath)

	const waiters = 12
	results := make(chan error, waiters)
	for i := range waiters {
		go func() {
			if i%2 == 0 {
				results <- supervisor.Wait()
				return
			}
			results <- supervisor.terminateAfter(5 * time.Second)
		}()
	}
	for range waiters {
		if err := <-results; err != nil {
			t.Fatalf("completion result: %v", err)
		}
	}
	for range 3 {
		if err := supervisor.Terminate(); err != nil {
			t.Fatalf("repeated Terminate: %v", err)
		}
	}
}

func TestCLIProcessSupervisorStartFailureIsStableCompletion(t *testing.T) {
	supervisor := NewCLIProcessSupervisor(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		filepath.Join(t.TempDir(), "missing"),
		nil,
	)
	startErr := supervisor.Start()
	if startErr == nil {
		t.Fatal("Start returned nil, want missing executable error")
	}
	if waitErr := supervisor.Wait(); waitErr != startErr {
		t.Fatalf("Wait error = %v, want stable Start error %v", waitErr, startErr)
	}
	if terminateErr := supervisor.Terminate(); terminateErr != startErr {
		t.Fatalf("Terminate error = %v, want stable Start error %v", terminateErr, startErr)
	}
}

func TestCLIProcessSupervisorCanceledStartIsStableCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	supervisor := NewCLIProcessSupervisor(ctx, logrus.NewEntry(logrus.New()), "unused", nil)

	startErr := supervisor.Start()
	if !errors.Is(startErr, context.Canceled) {
		t.Fatalf("Start error = %v, want context.Canceled", startErr)
	}
	if waitErr := supervisor.Wait(); waitErr != startErr {
		t.Fatalf("Wait error = %v, want stable Start error %v", waitErr, startErr)
	}
	if terminateErr := supervisor.Terminate(); terminateErr != startErr {
		t.Fatalf("Terminate error = %v, want stable Start error %v", terminateErr, startErr)
	}
}

func TestCLIProcessSupervisorWaitBeforeStartReceivesCompletion(t *testing.T) {
	supervisor := NewCLIProcessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "", nil)
	result := make(chan error, 1)
	go func() {
		result <- supervisor.Wait()
	}()

	if err := supervisor.startCommand(cliSubprocessSupervisorTestCommand(t, "exit", "0")); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestCLIProcessSupervisorTerminateBeforeStartCompletesLifecycle(t *testing.T) {
	supervisor := NewCLIProcessSupervisor(context.Background(), logrus.NewEntry(logrus.New()), "unused", nil)
	if err := supervisor.Terminate(); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if err := supervisor.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if err := supervisor.Start(); !errors.Is(err, ErrCLIProcessAlreadyStarted) {
		t.Fatalf("Start error = %v, want ErrCLIProcessAlreadyStarted", err)
	}
}

func TestCliSubprocessSupervisorHelper(t *testing.T) {
	if os.Getenv("BLDR_DEVTOOL_CLI_PROCESS_HELPER") != "1" {
		return
	}

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
	t.Helper()

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
	t.Helper()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	return executable
}

func cliSubprocessSupervisorTestArgs(mode string, args ...string) []string {
	return append([]string{"-test.run=TestCliSubprocessSupervisorHelper", "--", mode}, args...)
}

func cliSubprocessSupervisorHelperArgs() []string {
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	return nil
}

func markCliSubprocessHelperReady(path string) {
	if err := os.WriteFile(path, []byte("ready"), 0o644); err != nil {
		os.Exit(2)
	}
}

type cliSubprocessSupervisorCloseRecorder struct {
	once   sync.Once
	closed chan struct{}
}

func newCLIProcessSupervisorCloseRecorder() *cliSubprocessSupervisorCloseRecorder {
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
