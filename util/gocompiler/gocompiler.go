package gocompiler

import (
	"bytes"
	io "io"
	"os"
	"os/exec"
	"slices"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_platform_go "github.com/aperturerobotics/bldr/platform/go"
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

func NewGoCompilerCmd(args ...string) *exec.Cmd {
	ecmd := uexec.NewCmd("go", args...)
	ecmd.Env = os.Environ()
	ecmd.Env = append(
		ecmd.Env,
		GetDefaultEnv()...,
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

// NewBuildTags constructs build tags for a build type.
//
// NOTE: ExecBuildEntrypoint calls this automatically.
func NewBuildTags(buildPlatform bldr_platform.Platform, buildType bldr_manifest.BuildType, enableCgo bool) []string {
	buildTags := []string{"build_type_" + buildType.String()}
	if !enableCgo {
		buildTags = append(buildTags, "purego")
	}
	return buildTags
}

// ExecBuildEntrypoint executes building an entrypoint main package.
func ExecBuildEntrypoint(
	le *logrus.Entry,
	buildPlatform bldr_platform.Platform,
	buildType bldr_manifest.BuildType,
	workingPath,
	outBinPath string,
	enableCgo bool,
	buildTags []string,
	ldFlags []string,
) error {
	isNativeBuildPlatform := buildPlatform.GetBasePlatformID() == bldr_platform.PlatformID_NATIVE
	platformEnv, err := bldr_platform_go.PlatformToGoEnv(buildPlatform)
	if err != nil {
		return err
	}

	args := append([]string{
		"build",
		"-trimpath",
		"-o",
		outBinPath,
	}, GetDefaultArgs()...)

	// always disable cgo if not native platform
	if !isNativeBuildPlatform {
		enableCgo = false
	}

	// build tags
	buildTags = append(buildTags, NewBuildTags(buildPlatform, buildType, enableCgo)...)

	// add build tags to build args
	if len(buildTags) != 0 {
		args = append(args, "-tags="+strings.Join(buildTags, ","))
	}

	// if release or not native platform drop debugging symbols
	isRelease := buildType.IsRelease()
	if isRelease || !isNativeBuildPlatform {
		ldFlags = slices.Clone(ldFlags)
		ldFlags = append(ldFlags, "-w", "-s")
	}

	// ldflags
	if len(ldFlags) != 0 {
		args = append(args, "-ldflags", strings.Join(ldFlags, " "))
	}

	// module path
	args = append(args, ".")

	// go build
	ecmd := NewGoCompilerCmd(args...)
	ecmd.Dir = workingPath
	if enableCgo {
		ecmd.Env = append(ecmd.Env, "CGO_ENABLED=1")
	} else {
		ecmd.Env = append(ecmd.Env, "CGO_ENABLED=0")
	}
	ecmd.Env = append(ecmd.Env, platformEnv...)

	return ExecGoCompiler(le, ecmd)
}
