//go:build !js

package devtool

import (
	"context"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

const cliSubprocessKillTimeout = 3 * time.Second

type cliSubprocessSupervisor struct {
	ctx        context.Context
	le         *logrus.Entry
	binaryPath string
	args       []string

	killTimeout time.Duration
	cmd         *exec.Cmd
	stderr      io.Closer
	done        chan error
}

func newCliSubprocessSupervisor(
	ctx context.Context,
	le *logrus.Entry,
	binaryPath string,
	args []string,
) *cliSubprocessSupervisor {
	return &cliSubprocessSupervisor{
		ctx:         ctx,
		le:          le,
		binaryPath:  binaryPath,
		args:        args,
		killTimeout: cliSubprocessKillTimeout,
	}
}

func (s *cliSubprocessSupervisor) start() error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	cmd := exec.Command(s.binaryPath, s.args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = s.newStderrWriter()
	return s.startCommand(cmd)
}

func (s *cliSubprocessSupervisor) startCommand(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		s.closeStderr()
		return err
	}

	s.cmd = cmd
	s.done = make(chan error, 1)
	go func() {
		err := cmd.Wait()
		s.closeStderr()
		s.done <- err
	}()
	return nil
}

func (s *cliSubprocessSupervisor) wait() <-chan error {
	return s.done
}

func (s *cliSubprocessSupervisor) terminate() error {
	return s.terminateAfter(s.killTimeout)
}

func (s *cliSubprocessSupervisor) terminateAfter(timeout time.Duration) error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	_ = s.cmd.Process.Signal(syscall.SIGTERM)
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-s.done:
		return err
	case <-timer.C:
		_ = s.cmd.Process.Kill()
		return <-s.done
	}
}

func (s *cliSubprocessSupervisor) newStderrWriter() io.Writer {
	stderr := s.le.WriterLevel(logrus.DebugLevel)
	s.stderr = stderr
	return stderr
}

func (s *cliSubprocessSupervisor) closeStderr() {
	if s.stderr != nil {
		_ = s.stderr.Close()
		s.stderr = nil
	}
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
