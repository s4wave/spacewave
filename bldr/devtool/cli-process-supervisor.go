//go:build !js

package devtool

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

const cliSubprocessKillTimeout = 3 * time.Second

// ErrCLIProcessAlreadyStarted is returned when Start is called more than once
// or after the supervisor has been terminated.
var ErrCLIProcessAlreadyStarted = errors.New("CLI process supervisor already started")

type cliProcessState uint8

const (
	cliProcessReady cliProcessState = iota
	cliProcessStarted
	cliProcessDone
)

// CLIProcessSupervisor runs one CLI subprocess and owns its completion and termination.
type CLIProcessSupervisor struct {
	// ctx prevents a subprocess from starting after its caller is canceled.
	ctx context.Context
	// le receives the subprocess stderr stream.
	le *logrus.Entry
	// binaryPath is the configured executable path.
	binaryPath string
	// args are the configured executable arguments.
	args []string

	// killTimeout bounds graceful termination before a forced kill.
	killTimeout time.Duration
	// mtx guards the subprocess lifecycle and result.
	mtx sync.Mutex
	// state records the single start attempt and its completion.
	state cliProcessState
	// cmd is the running subprocess after Start succeeds.
	cmd *exec.Cmd
	// stderr is closed after start failure or subprocess exit.
	stderr io.Closer
	// result is stable after done closes.
	result error
	// done closes when the start attempt or subprocess completes.
	done chan struct{}
	// terminateOnce sends at most one graceful signal and forced kill.
	terminateOnce sync.Once
}

// NewCLIProcessSupervisor constructs a supervisor for one configured CLI subprocess.
func NewCLIProcessSupervisor(
	ctx context.Context,
	le *logrus.Entry,
	binaryPath string,
	args []string,
) *CLIProcessSupervisor {
	return &CLIProcessSupervisor{
		ctx:         ctx,
		le:          le,
		binaryPath:  binaryPath,
		args:        args,
		killTimeout: cliSubprocessKillTimeout,
		done:        make(chan struct{}),
	}
}

// Start launches the subprocess. The supervisor accepts exactly one start attempt.
func (s *CLIProcessSupervisor) Start() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.state != cliProcessReady {
		return ErrCLIProcessAlreadyStarted
	}
	s.state = cliProcessStarted
	if err := s.ctx.Err(); err != nil {
		s.completeLocked(err)
		return err
	}

	// The supervisor launches the devtool-configured CLI binary with its own
	// configured arguments, not caller-supplied input.
	cmd := exec.Command(s.binaryPath, s.args...) //nolint:gosec // G204: binary path and args are devtool-owned config
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = s.newStderrWriterLocked()
	return s.startCommandLocked(cmd)
}

func (s *CLIProcessSupervisor) startCommand(cmd *exec.Cmd) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.state != cliProcessReady {
		return ErrCLIProcessAlreadyStarted
	}
	s.state = cliProcessStarted
	if err := s.ctx.Err(); err != nil {
		s.completeLocked(err)
		return err
	}
	return s.startCommandLocked(cmd)
}

func (s *CLIProcessSupervisor) startCommandLocked(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		s.completeLocked(err)
		return err
	}

	s.cmd = cmd
	go func() {
		s.complete(cmd.Wait())
	}()
	return nil
}

// Done closes when the start attempt or subprocess completes.
func (s *CLIProcessSupervisor) Done() <-chan struct{} {
	return s.done
}

// Wait blocks until completion and returns the stable start or subprocess result.
func (s *CLIProcessSupervisor) Wait() error {
	<-s.done
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.result
}

// Terminate requests graceful termination, kills after the configured timeout, and waits.
func (s *CLIProcessSupervisor) Terminate() error {
	return s.terminateAfter(s.killTimeout)
}

func (s *CLIProcessSupervisor) terminateAfter(timeout time.Duration) error {
	s.terminateOnce.Do(func() {
		s.mtx.Lock()
		if s.state == cliProcessReady {
			s.state = cliProcessDone
			close(s.done)
			s.mtx.Unlock()
			return
		}
		if s.state == cliProcessDone {
			s.mtx.Unlock()
			return
		}
		cmd := s.cmd
		s.mtx.Unlock()

		_ = cmd.Process.Signal(syscall.SIGTERM)
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-s.done:
		case <-timer.C:
			_ = cmd.Process.Kill()
			<-s.done
		}
	})
	return s.Wait()
}

func (s *CLIProcessSupervisor) newStderrWriterLocked() io.Writer {
	stderr := s.le.WriterLevel(logrus.DebugLevel)
	s.stderr = stderr
	return stderr
}

func (s *CLIProcessSupervisor) complete(err error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.completeLocked(err)
}

func (s *CLIProcessSupervisor) completeLocked(err error) {
	if s.state == cliProcessDone {
		return
	}
	s.state = cliProcessDone
	s.result = err
	if s.stderr != nil {
		_ = s.stderr.Close()
		s.stderr = nil
	}
	close(s.done)
}

// exitWithChildCode propagates a CLI subprocess exit to the devtool process.
// It returns nil for success and a non-ExitError unchanged, but for an
// *exec.ExitError it terminates the process via os.Exit with the child's exit
// code and does not return, so the devtool exit status mirrors the child.
func exitWithChildCode(err error) error {
	if err == nil {
		return nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		os.Exit(exitErr.ExitCode())
	}
	return err
}
