package exec

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"

	uexec "github.com/aperturerobotics/util/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// NewCmd builds a new exec cmd with defaults.
var NewCmd = uexec.NewCmd

// StartAndWait runs the given process and waits for ctx or process to complete.
// On process failure it returns an error carrying the full captured stderr, not
// just the trailing line: tool stack traces (Rolldown/Oxc, bun) put the real
// message at the top and a bare frame such as "at processTicksAndRejections" at
// the bottom, so the upstream last-line-only interpretation swallows the cause.
func StartAndWait(ctx context.Context, le *logrus.Entry, ecmd *exec.Cmd) error {
	var stderrBuf bytes.Buffer
	if ecmd.Process == nil {
		stderrLogger := le.WriterLevel(logrus.DebugLevel)
		stdoutLogger := le.WriterLevel(logrus.DebugLevel)
		ecmd.Stderr = io.MultiWriter(&stderrBuf, stderrLogger)
		if ecmd.Stdout == nil || writerIsStdout(ecmd.Stdout) {
			ecmd.Stdout = stdoutLogger
		}
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
		if err == nil {
			le.Debug("process exited")
			return nil
		}
		le.WithError(err).Debug("process exited with error")
		stderr := strings.TrimRight(stderrBuf.String(), "\n")
		if stderr == "" {
			return err
		}
		return errors.Errorf("%s: %s", err, stderr)
	}
}

func writerIsStdout(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && file == os.Stdout
}
