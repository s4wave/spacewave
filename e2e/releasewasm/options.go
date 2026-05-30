//go:build !js

package releasewasm

import (
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
)

const (
	// E2EReleaseWasmTinyGoProfileEnv selects the release-wasm TinyGo build profile.
	E2EReleaseWasmTinyGoProfileEnv = "E2E_RELEASE_WASM_TINYGO_PROFILE"
	// E2EReleaseWasmTinyGoOptEnv overrides the release-wasm TinyGo optimization level.
	E2EReleaseWasmTinyGoOptEnv = "E2E_RELEASE_WASM_TINYGO_OPT"
	// E2EReleaseWasmTinyGoPanicEnv overrides the release-wasm TinyGo panic strategy.
	E2EReleaseWasmTinyGoPanicEnv = "E2E_RELEASE_WASM_TINYGO_PANIC"
	// E2EReleaseWasmTinyGoGCEnv overrides the release-wasm TinyGo garbage collector.
	E2EReleaseWasmTinyGoGCEnv = "E2E_RELEASE_WASM_TINYGO_GC"
	// E2EReleaseWasmTinyGoSchedulerEnv overrides the release-wasm TinyGo scheduler.
	E2EReleaseWasmTinyGoSchedulerEnv = "E2E_RELEASE_WASM_TINYGO_SCHEDULER"
	// E2EReleaseWasmTinyGoStackSizeEnv overrides the release-wasm TinyGo stack size.
	E2EReleaseWasmTinyGoStackSizeEnv = "E2E_RELEASE_WASM_TINYGO_STACK_SIZE"
	// E2EReleaseWasmTinyGoLLVMFeaturesEnv overrides the release-wasm TinyGo LLVM feature set.
	E2EReleaseWasmTinyGoLLVMFeaturesEnv = "E2E_RELEASE_WASM_TINYGO_LLVM_FEATURES"
	// E2EReleaseWasmTinyGoInterpTimeoutEnv overrides release-wasm TinyGo interp timeout.
	E2EReleaseWasmTinyGoInterpTimeoutEnv = "E2E_RELEASE_WASM_TINYGO_INTERP_TIMEOUT"
	// E2EReleaseWasmTinyGoDebugInfoEnv keeps TinyGo debug info for release-wasm diagnostics.
	E2EReleaseWasmTinyGoDebugInfoEnv = "E2E_RELEASE_WASM_TINYGO_DEBUG"
)

func applyReleaseWasmTinyGoCompilerEnv() error {
	profile := strings.TrimSpace(os.Getenv(E2EReleaseWasmTinyGoProfileEnv))
	if profile == "" {
		profile = strings.TrimSpace(os.Getenv(gocompiler.TinyGoProfileEnv))
	}
	if profile == "" {
		profile = gocompiler.TinyGoProfileFast
	}
	if err := os.Setenv(gocompiler.TinyGoProfileEnv, profile); err != nil {
		return errors.Wrap(err, "set TinyGo profile")
	}

	if err := copyOptionalReleaseWasmTinyGoEnv(E2EReleaseWasmTinyGoOptEnv, gocompiler.TinyGoOptEnv); err != nil {
		return err
	}
	if err := copyOptionalReleaseWasmTinyGoEnv(E2EReleaseWasmTinyGoPanicEnv, gocompiler.TinyGoPanicStrategyEnv); err != nil {
		return err
	}
	if err := copyOptionalReleaseWasmTinyGoEnv(E2EReleaseWasmTinyGoGCEnv, gocompiler.TinyGoGCEnv); err != nil {
		return err
	}
	if profile == gocompiler.TinyGoProfileFast && strings.TrimSpace(os.Getenv(gocompiler.TinyGoGCEnv)) == "" {
		if err := os.Setenv(gocompiler.TinyGoGCEnv, "leaking"); err != nil {
			return errors.Wrap(err, "set TinyGo GC")
		}
	}
	if err := copyOptionalReleaseWasmTinyGoEnv(E2EReleaseWasmTinyGoSchedulerEnv, gocompiler.TinyGoSchedulerEnv); err != nil {
		return err
	}
	if err := copyOptionalReleaseWasmTinyGoEnv(E2EReleaseWasmTinyGoStackSizeEnv, gocompiler.TinyGoStackSizeEnv); err != nil {
		return err
	}
	if err := copyOptionalReleaseWasmTinyGoEnv(E2EReleaseWasmTinyGoLLVMFeaturesEnv, gocompiler.TinyGoLLVMFeaturesEnv); err != nil {
		return err
	}
	if err := copyOptionalReleaseWasmTinyGoEnv(E2EReleaseWasmTinyGoInterpTimeoutEnv, gocompiler.TinyGoInterpTimeoutEnv); err != nil {
		return err
	}
	if err := copyOptionalReleaseWasmTinyGoEnv(E2EReleaseWasmTinyGoDebugInfoEnv, gocompiler.TinyGoDebugInfoEnv); err != nil {
		return err
	}

	if _, err := gocompiler.GetDefaultTinygoArgs(); err != nil {
		return err
	}
	if _, err := gocompiler.TinyGoDebugInfoEnabled(); err != nil {
		return err
	}
	return nil
}

func copyOptionalReleaseWasmTinyGoEnv(src, dst string) error {
	value := strings.TrimSpace(os.Getenv(src))
	if value == "" {
		return nil
	}
	if err := os.Setenv(dst, value); err != nil {
		return errors.Wrapf(err, "set %s from %s", dst, src)
	}
	return nil
}
