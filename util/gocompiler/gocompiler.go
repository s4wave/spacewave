package gocompiler

import (
	"bytes"
	io "io"
	"os"
	"os/exec"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	uexec "github.com/aperturerobotics/util/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// GetDefaultArgs are the set of args we usually pass to the compiler.
func GetDefaultArgs() []string {
	return []string{
		"-v",
		"-buildvcs=false",
		"-mod=readonly",
	}
}

// GetDefaultEnv are the set of args we usually pass to the compiler.
func GetDefaultEnv() []string {
	return []string{
		"GO111MODULE=on",
		"GOPROXY=direct",
		// required for -mod=vendor
		"GOWORK=off",
	}
}

func NewGoCompilerCmd(cmd string, args ...string) *exec.Cmd {
	ecmd := uexec.NewCmd(cmd, args...)
	ecmd.Env = os.Environ()
	ecmd.Env = append(ecmd.Env, GetDefaultEnv()...)
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

// NewBuildTags constructs build tags for a build type.
//
// NOTE: ExecBuildEntrypoint calls this automatically.
func NewBuildTags(buildType bldr_manifest.BuildType, enableCgo bool) []string {
	buildTags := []string{"build_type_" + buildType.String()}
	if !enableCgo {
		buildTags = append(buildTags, "purego")
	}
	return buildTags
}
