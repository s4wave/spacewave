//go:build !js

package releasewasm

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/bldr/util/gocompiler"
)

func TestApplyReleaseWasmTinyGoCompilerEnvDefaultsToFastProfile(t *testing.T) {
	clearBldrTinyGoEnv(t)
	clearReleaseWasmTinyGoEnv(t)

	if err := applyReleaseWasmTinyGoCompilerEnv(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(gocompiler.TinyGoProfileEnv); got != gocompiler.TinyGoProfileFast {
		t.Fatalf("%s=%q, want %q", gocompiler.TinyGoProfileEnv, got, gocompiler.TinyGoProfileFast)
	}
	args, err := gocompiler.GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "-opt=1") {
		t.Fatalf("tinygo args = %v, want -opt=1", args)
	}
	if slices.Contains(args, "-opt=0") {
		t.Fatalf("release-wasm TinyGo profile should not use broken -opt=0: %v", args)
	}
	if !slices.Contains(args, "-interp-timeout=10m") {
		t.Fatalf("tinygo args = %v, want -interp-timeout=10m", args)
	}
	if !slices.Contains(args, "-gc=leaking") {
		t.Fatalf("tinygo args = %v, want -gc=leaking", args)
	}
	if got := os.Getenv(gocompiler.TinyGoGCEnv); got != "leaking" {
		t.Fatalf("%s=%q, want leaking", gocompiler.TinyGoGCEnv, got)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-scheduler=") {
			t.Fatalf("release-wasm TinyGo profile should not set scheduler by default: %v", args)
		}
	}
}

func TestApplyReleaseWasmTinyGoCompilerEnvPreservesGlobalProfile(t *testing.T) {
	clearBldrTinyGoEnv(t)
	clearReleaseWasmTinyGoEnv(t)
	t.Setenv(gocompiler.TinyGoProfileEnv, gocompiler.TinyGoProfileOptimized)

	if err := applyReleaseWasmTinyGoCompilerEnv(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(gocompiler.TinyGoProfileEnv); got != gocompiler.TinyGoProfileOptimized {
		t.Fatalf("%s=%q, want %q", gocompiler.TinyGoProfileEnv, got, gocompiler.TinyGoProfileOptimized)
	}
	args, err := gocompiler.GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "-opt=2") {
		t.Fatalf("tinygo args = %v, want -opt=2", args)
	}
}

func TestApplyReleaseWasmTinyGoCompilerEnvCopiesExplicitKnobs(t *testing.T) {
	clearBldrTinyGoEnv(t)
	clearReleaseWasmTinyGoEnv(t)
	t.Setenv(E2EReleaseWasmTinyGoProfileEnv, gocompiler.TinyGoProfileFast)
	t.Setenv(E2EReleaseWasmTinyGoSchedulerEnv, "none")
	t.Setenv(E2EReleaseWasmTinyGoStackSizeEnv, "512KB")
	t.Setenv(E2EReleaseWasmTinyGoInterpTimeoutEnv, "12m")
	t.Setenv(E2EReleaseWasmTinyGoDebugInfoEnv, "true")

	if err := applyReleaseWasmTinyGoCompilerEnv(); err != nil {
		t.Fatal(err)
	}
	for env, want := range map[string]string{
		gocompiler.TinyGoSchedulerEnv:     "none",
		gocompiler.TinyGoStackSizeEnv:     "512KB",
		gocompiler.TinyGoInterpTimeoutEnv: "12m",
		gocompiler.TinyGoDebugInfoEnv:     "true",
	} {
		if got := os.Getenv(env); got != want {
			t.Fatalf("%s=%q, want %q", env, got, want)
		}
	}
}

func clearBldrTinyGoEnv(t *testing.T) {
	t.Helper()
	for _, key := range gocompiler.TinyGoStartupCacheEnvKeys() {
		t.Setenv(key, "")
	}
}

func clearReleaseWasmTinyGoEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		E2EReleaseWasmTinyGoProfileEnv,
		E2EReleaseWasmTinyGoOptEnv,
		E2EReleaseWasmTinyGoPanicEnv,
		E2EReleaseWasmTinyGoGCEnv,
		E2EReleaseWasmTinyGoSchedulerEnv,
		E2EReleaseWasmTinyGoStackSizeEnv,
		E2EReleaseWasmTinyGoLLVMFeaturesEnv,
		E2EReleaseWasmTinyGoInterpTimeoutEnv,
		E2EReleaseWasmTinyGoDebugInfoEnv,
	} {
		t.Setenv(key, "")
	}
}
