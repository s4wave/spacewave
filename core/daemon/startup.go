//go:build !js

package daemon

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aperturerobotics/util/pipesock"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/util/randstring"
	"github.com/sirupsen/logrus"
)

const (
	// StartupTimeoutEnvVar selects the startup and Resource Init deadline.
	StartupTimeoutEnvVar = "SPACEWAVE_DAEMON_STARTUP_TIMEOUT"
	// TracePathEnvVar forwards the optional daemon runtime trace path.
	TracePathEnvVar = "SPACEWAVE_DAEMON_TRACE"
	// DefaultStartupTimeout bounds startup without expiring retained clients.
	DefaultStartupTimeout = time.Minute
)

// Launcher records what started a detached daemon. The daemon reads it to
// decide whether to yield its state path to a manually started serve.
type Launcher string

const (
	// LauncherManual marks a daemon started by hand.
	LauncherManual Launcher = ""
	// LauncherCommand marks a daemon started on demand for a command. It
	// yields to a manually started serve.
	LauncherCommand Launcher = "command"
	// LauncherDesktop marks a daemon started by the desktop app.
	LauncherDesktop Launcher = "desktop"
)

// StartupTimeout returns the configured startup deadline duration.
func StartupTimeout() (time.Duration, error) {
	// Reject malformed or nonpositive startup deadlines before creating a child.
	raw := os.Getenv(StartupTimeoutEnvVar)
	if raw == "" {
		return DefaultStartupTimeout, nil
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.Wrap(err, StartupTimeoutEnvVar)
	}
	if dur <= 0 {
		return 0, errors.New(StartupTimeoutEnvVar + " must be positive")
	}
	return dur, nil
}

// ServeArgs returns the current executable's detached daemon invocation.
func ServeArgs(statePath, pipeID string, launcher Launcher) []string {
	// Preserve inherited logging and storage settings; trace is an explicit flag.
	args := []string{"--state-path", statePath, "serve", "--daemon-startup-pipe-id", pipeID}
	if launcher != LauncherManual {
		args = append(args, "--daemon-launcher", string(launcher))
	}
	if tracePath := os.Getenv(TracePathEnvVar); tracePath != "" {
		args = append(args, "--trace", tracePath)
	}
	return args
}

// NewStartupPipeLogger suppresses transport diagnostics on the private pipe.
func NewStartupPipeLogger() *logrus.Entry {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return logrus.NewEntry(logger)
}

// StartProcess starts the current executable on demand for a command, with
// StartExecutable's readiness and child-custody guarantees.
func StartProcess(ctx context.Context, statePath string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return StartExecutable(ctx, statePath, executable, LauncherCommand)
}

// StartExecutable starts the selected executable and transfers custody before
// returning success. Failure stops and joins only this attempt's unready child.
// If the OS refuses termination without a verified exit, failure releases the
// handles and reports that the unacknowledged child may remain alive. It never
// requests shutdown through the shared socket. launcher tells the daemon what
// started it.
func StartExecutable(
	ctx context.Context,
	statePath string,
	executable string,
	launcher Launcher,
) error {
	// Establish the private startup channel before creating the child.
	timeout, err := StartupTimeout()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	pipeID := "sw-" + randstring.RandomIdentifier(6)
	listener, err := pipesock.BuildPipeListener(NewStartupPipeLogger(), statePath, pipeID)
	if err != nil {
		return errors.Wrap(err, "listen for daemon startup")
	}
	defer listener.Close()

	// Launch the selected composition with the complete inherited environment.
	// #nosec G204 -- callers select the executable; desktop verifies its copy before launch.
	cmd := exec.Command(executable, ServeArgs(statePath, pipeID, launcher)...)
	if err := prepareDaemonStart(cmd); err != nil {
		return err
	}
	nullFile, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer nullFile.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nullFile, nullFile, nullFile
	process, err := startProcess(cmd)
	if err != nil {
		return err
	}

	// The child cannot serve clients until WaitStartup writes its acknowledgement.
	if err := WaitStartup(ctx, listener); err != nil {
		if stopErr := process.stop(); stopErr != nil {
			return errors.Wrapf(stopErr, "daemon startup failed (%v); stop unready child", err)
		}
		return err
	}
	return process.release()
}

// WaitStartup acknowledges a ready child's transfer to independent lifetime.
// A failed return guarantees that no acknowledgement was written. The child
// must not accept public clients until it receives that acknowledgement.
func WaitStartup(ctx context.Context, listener net.Listener) error {
	// Cancel both accept and read without leaving a startup reader behind.
	stopAccept := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stopAccept()
	conn, err := listener.Accept()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	defer conn.Close()
	stopRead := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopRead()

	// A one-byte acknowledgement fences public service from parent cleanup.
	message, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	message = strings.TrimSpace(message)
	switch {
	case message == "ready":
		n, err := conn.Write([]byte{1})
		if n == 1 {
			return nil
		}
		return errors.Wrap(err, "acknowledge daemon readiness")
	case message == "starting":
		return ErrStarting
	case strings.HasPrefix(message, "error: "):
		return errors.New(strings.TrimPrefix(message, "error: "))
	default:
		return errors.Errorf("unexpected daemon startup status %q", message)
	}
}
