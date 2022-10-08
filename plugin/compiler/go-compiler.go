package plugin_compiler

import (
	"bytes"
	io "io"
	"os/exec"
	"strings"

	uexec "github.com/aperturerobotics/controllerbus/util/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func NewGoCompilerCmd(args ...string) *exec.Cmd {
	ecmd := uexec.NewCmd("go", args...)
	ecmd.Env = append(
		ecmd.Env,
		"GO111MODULE=on",
	)

	return ecmd
}

// ExecGoCompiler runs the Go compiler and collects the log output.
func ExecGoCompiler(le *logrus.Entry, cmd *exec.Cmd) error {
	var stderrBuf bytes.Buffer

	goLogger := le.WriterLevel(logrus.DebugLevel)
	cmd.Stderr = io.MultiWriter(&stderrBuf, goLogger)
	le.
		WithField("work-dir", cmd.Dir).
		Debugf("running go compiler: %s", cmd.String())

	err := cmd.Run()
	if err != nil && (strings.HasPrefix(err.Error(), "exit status") || strings.HasPrefix(err.Error(), "err: exit status")) {
		stderrLines := strings.Split(stderrBuf.String(), "\n")
		errMsg := stderrLines[len(stderrLines)-1]
		if len(errMsg) == 0 && len(stderrLines) > 1 {
			errMsg = stderrLines[len(stderrLines)-2]
		}
		err = errors.New(errMsg)
	}
	return err
}
