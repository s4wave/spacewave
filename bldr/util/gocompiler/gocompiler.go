package gocompiler

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	uexec "github.com/aperturerobotics/util/exec"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/sirupsen/logrus"
)

// BldrTinyGoJSImportBuildTag selects Bldr's direct TinyGo JavaScript import ABI.
const BldrTinyGoJSImportBuildTag = "bldr_tinygo_js_imports"

// GoScriptBuildTag selects source meant for GoScript compilation.
const GoScriptBuildTag = "goscript"

// CloudflareBuildTag selects source meant for Cloudflare Workers builds.
const CloudflareBuildTag = "bldr_cloudflare"

// SQLLiteBuildTag selects the reduced SQL surface for browser builds: source
// gated on sql_lite drops the go-mysql-server driver, stdlib database/sql
// integration, and other native-only SQL paths from the bundle.
const SQLLiteBuildTag = "sql_lite"

// PureGoBuildTag selects portable Go over assembly in the standard library and
// dependencies. Only targets that cannot run Go assembly set it: TinyGo,
// GoScript, and WebAssembly output. Native builds keep hardware hashing.
const PureGoBuildTag = "purego"

// GoScriptCompilerCacheRootEnv opts Bldr GoScript compiles into the compiler
// package artifact cache.
const GoScriptCompilerCacheRootEnv = "BLDR_GOSCRIPT_COMPILER_CACHE_ROOT"

// GetDefaultArgs returns compiler flags that preserve the module manifest.
func GetDefaultArgs() []string {
	return []string{
		"-v",
		"-buildvcs=false",
		"-mod=readonly",
	}
}

// GetDefaultTinygoLlvmFeatures returns the WebAssembly features enabled for TinyGo.
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

// GetDefaultEnv selects module mode without changing the configured module proxy.
func GetDefaultEnv() []string {
	return []string{
		"GO111MODULE=on",
		// Vendored builds require module isolation from workspace configuration.
		"GOWORK=off",
	}
}

// NewGoCompilerCmd builds a compiler command with module defaults and the caller's environment.
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
func NewBuildTags(buildType bldr_manifest.BuildType) []string {
	return []string{"build_type_" + buildType.String()}
}

// GetWasmExecPath gets the path to wasm_exec.js and ensures it exists.
func GetWasmExecPath(ctx context.Context, le *logrus.Entry, useTinygo bool) (string, error) {
	// Query the selected compiler's installation root.
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
	goRootDir, _, _ := strings.Cut(gocBuf.String(), "\n")

	// Resolve the runtime adapter and require it to exist in that installation.
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
