package exec

import (
	"bytes"
	"context"
	"io"
	"os/exec"

	uexec "github.com/aperturerobotics/util/exec"
	"github.com/sirupsen/logrus"
)

// NewCmd builds a new exec cmd with defaults.
var NewCmd = uexec.NewCmd

// StartAndWait runs the given process and waits for ctx or process to complete.
// Unlike the upstream util/exec.StartAndWait, this includes stderr context in
// error messages via InterpretCmdErr.
func StartAndWait(ctx context.Context, le *logrus.Entry, ecmd *exec.Cmd) error {
	var stderrBuf bytes.Buffer
	if ecmd.Process == nil {
		goLogger := le.WriterLevel(logrus.DebugLevel)
		ecmd.Stderr = io.MultiWriter(&stderrBuf, goLogger)
		le.WithField("work-dir", ecmd.Dir).
			Debugf("running command: %s", ecmd.String())
		if err := ecmd.Start(); err != nil {
			return err
		}
	}

	outErr := make(chan error, 1)
	go func() {
		outErr <- ecmd.Wait()
	}()

	select {
	case <-ctx.Done():
		_ = ecmd.Process.Kill()
		<-outErr
		return ctx.Err()
	case err := <-outErr:
		le := le.WithField("exit-code", ecmd.ProcessState.ExitCode())
		if err != nil {
			le.WithError(err).Debug("process exited with error")
			return uexec.InterpretCmdErr(err, stderrBuf)
		}
		le.Debug("process exited")
		return nil
	}
}
