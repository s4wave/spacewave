package gocompiler

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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

// GetDefaultTinygoArgs are the set of args we usually pass to the compiler.
func GetDefaultTinygoArgs() []string {
	return []string{
		"-opt=2",
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
	return uexec.ExecCmd(le, cmd)
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

// GetWasmExecPath gets the path to wasm_exec.js and ensures it exists.
func GetWasmExecPath(le *logrus.Entry, useTinygo bool) (string, error) {
	var goc *exec.Cmd
	if useTinygo {
		goc = NewGoCompilerCmd("tinygo", "env", "TINYGOROOT")
	} else {
		goc = NewGoCompilerCmd("go", "env", "GOROOT")
	}

	var gocBuf bytes.Buffer
	goc.Stdout = &gocBuf
	if err := uexec.ExecCmd(le, goc); err != nil {
		return "", errors.Wrap(err, "cannot determine GOROOT")
	}
	goRootDir := strings.SplitN(gocBuf.String(), "\n", 2)[0]

	var wasmExecFile string
	if useTinygo {
		wasmExecFile = filepath.Join(goRootDir, "targets/wasm_exec.js")
	} else {
		wasmExecFile = filepath.Join(goRootDir, "misc/wasm/wasm_exec.js")
	}

	if _, err := os.Stat(wasmExecFile); err != nil {
		return wasmExecFile, errors.Wrapf(err, "cannot find wasm_exec.js in goroot: %s", wasmExecFile)
	}
	return wasmExecFile, nil
}
