package gocompiler

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
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

// GetDefaultTinygoLlvmFeatures are the set of additional features to enable or disable.
func GetDefaultTinygoLlvmFeatures() []string {
	// https://github.com/llvm/llvm-project/blob/91423d71938d7a1dba27188e6d854148a750a3dd/clang/lib/Basic/Targets/WebAssembly.cpp#L150
	// https://github.com/llvm/llvm-project/blob/91423d71938d7a1dba27188e6d854148a750a3dd/clang/lib/Basic/Targets/WebAssembly.cpp#L180
	return []string{
		// https://caniuse.com/?search=WebAssembly
		// Baseline 2023: https://caniuse.com/wasm-simd
		"+simd128",
		// All browsers support: https://caniuse.com/wasm-signext
		"+sign-ext",
		// All browsers support: https://caniuse.com/wasm-threads
		"+atomics",
		// All browsers support: https://caniuse.com/wasm-bulk-memory
		"+bulk-memory",
		// All browsers support: https://caniuse.com/wasm-multi-value
		"+multivalue",
		// All browsers support: https://caniuse.com/wasm-mutable-globals
		"+mutable-globals",
		// All browsers support: https://caniuse.com/wasm-reference-types
		"+reference-types",
		// All browsers support: https://caniuse.com/wasm-nontrapping-fptoint
		"+nontrapping-fptoint",
	}
}

// GetDefaultTinygoArgs are the set of args we usually pass to the compiler.
func GetDefaultTinygoArgs() []string {
	return []string{
		"-opt=2",
		"-llvm-features=" + strings.Join(GetDefaultTinygoLlvmFeatures(), ","),
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

func NewGoCompilerCmd(ctx context.Context, cmd string, args ...string) *exec.Cmd {
	ecmd := uexec.NewCmd(ctx, cmd, args...)
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
func GetWasmExecPath(ctx context.Context, le *logrus.Entry, useTinygo bool) (string, error) {
	var goc *exec.Cmd
	if useTinygo {
		goc = NewGoCompilerCmd(ctx, "tinygo", "env", "TINYGOROOT")
	} else {
		goc = NewGoCompilerCmd(ctx, "go", "env", "GOROOT")
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
		wasmExecFile = filepath.Join(goRootDir, "lib/wasm/wasm_exec.js")
	}

	if _, err := os.Stat(wasmExecFile); err != nil {
		return wasmExecFile, errors.Wrapf(err, "cannot find wasm_exec.js in goroot: %s", wasmExecFile)
	}
	return wasmExecFile, nil
}
