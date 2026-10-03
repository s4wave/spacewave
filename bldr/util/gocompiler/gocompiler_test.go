package gocompiler

import (
	"slices"
	"strings"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
)

func TestNewBuildTagsDoNotDependOnReleaseEnv(t *testing.T) {
	for _, env := range []string{"", "prod", "staging"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("SPACEWAVE_RELEASE_ENV", env)
			tags := NewBuildTags(bldr_manifest.BuildType_RELEASE)
			if !slices.Equal(tags, []string{"build_type_release"}) {
				t.Fatalf("build tags = %v, want release defaults only", tags)
			}
		})
	}
}

func TestDefaultTinyGoArgsPrintPanic(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)

	// Resolve the compiler arguments for the configured TinyGo policy.
	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the required TinyGo compiler arguments.
	if !slices.Contains(args, "-panic=print") {
		t.Fatalf("tinygo args = %v, want -panic=print", args)
	}
	if !slices.Contains(args, "-stack-size="+TinyGoDefaultStackSize) {
		t.Fatalf("tinygo args = %v, want default stack size", args)
	}
}

func TestFastTinyGoProfileDropsBrokenOptZero(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoProfileEnv, TinyGoProfileFast)

	// Resolve the compiler arguments for the configured TinyGo policy.
	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the required TinyGo compiler arguments.
	if !slices.Contains(args, "-opt=1") {
		t.Fatalf("tinygo args = %v, want -opt=1", args)
	}

	// Verify the excluded TinyGo compiler arguments.
	if slices.Contains(args, "-opt=0") {
		t.Fatalf("fast TinyGo profile should not use broken -opt=0 by default: %v", args)
	}
	if !slices.Contains(args, "-interp-timeout=10m") {
		t.Fatalf("tinygo args = %v, want -interp-timeout=10m", args)
	}
	if !slices.Contains(args, "-gc=leaking") {
		t.Fatalf("tinygo args = %v, want -gc=leaking", args)
	}

	// Verify that TinyGo omits the unconfigured compiler option.
	for _, arg := range args {
		if strings.HasPrefix(arg, "-scheduler=") {
			t.Fatalf("fast TinyGo profile should not set scheduler by default: %v", args)
		}
	}
}

func TestOptimizedTinyGoProfileUsesOptTwo(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoProfileEnv, TinyGoProfileOptimized)

	// Resolve the compiler arguments for the configured TinyGo policy.
	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the required TinyGo compiler arguments.
	if !slices.Contains(args, "-opt=2") {
		t.Fatalf("tinygo args = %v, want -opt=2", args)
	}
}

func TestTinyGoSchedulerIsExplicit(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoProfileEnv, TinyGoProfileFast)
	t.Setenv(TinyGoSchedulerEnv, "none")

	// Resolve the compiler arguments for the configured TinyGo policy.
	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the required TinyGo compiler arguments.
	if !slices.Contains(args, "-scheduler=none") {
		t.Fatalf("tinygo args = %v, want -scheduler=none", args)
	}
}

func TestTinyGoStackSizeIsConfigurable(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoStackSizeEnv, "512KB")

	// Resolve the compiler arguments for the configured TinyGo policy.
	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the required TinyGo compiler arguments.
	if !slices.Contains(args, "-stack-size=512KB") {
		t.Fatalf("tinygo args = %v, want configured stack size", args)
	}
}

func TestTinyGoStackSizeCanUseTargetDefault(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoStackSizeEnv, "default")

	// Resolve the compiler arguments for the configured TinyGo policy.
	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}

	// Verify that TinyGo omits the unconfigured compiler option.
	for _, arg := range args {
		if strings.HasPrefix(arg, "-stack-size=") {
			t.Fatalf("tinygo args = %v, want target default stack size", args)
		}
	}
}

func TestTinyGoStackSizeRejectsUnsafeValue(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoStackSizeEnv, "512 KB")

	// Resolve the compiler arguments for the configured TinyGo policy.
	_, err := GetDefaultTinygoArgs()
	if err == nil {
		t.Fatal("expected invalid TinyGo stack size to fail")
	}

	// Verify that the compiler error identifies the invalid setting.
	if !strings.Contains(err.Error(), TinyGoStackSizeEnv) {
		t.Fatalf("error = %q, want %s", err.Error(), TinyGoStackSizeEnv)
	}
}

func TestTinyGoBrowserReleaseArgsIncludeNoDebugAndNoDWARF(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoProfileEnv, TinyGoProfileFast)

	// Resolve the compiler arguments for the configured TinyGo policy.
	platform := parseTestPlatform(t, "web/js/wasm")
	args, err := newTinyGoBuildArgs(platform, bldr_manifest.BuildType_RELEASE, "spacewave-core.wasm", []string{"build_type_release", "purego"})
	if err != nil {
		t.Fatal(err)
	}

	// Verify every required TinyGo compiler argument.
	for _, want := range []string{
		"-target",
		"wasm",
		"-opt=1",
		"-gc=leaking",
		"-stack-size=" + TinyGoDefaultStackSize,
		"-interp-timeout=10m",
		"-no-debug",
		tinyGoInternalNoDWARFArg,
		"-tags=build_type_release purego " + BldrTinyGoJSImportBuildTag,
	} {

		// Verify the required TinyGo compiler arguments.
		if !slices.Contains(args, want) {
			t.Fatalf("tinygo release args = %v, want %s", args, want)
		}
	}

	// Verify that TinyGo omits the unconfigured compiler option.
	for _, arg := range args {
		if strings.HasPrefix(arg, "-scheduler=") {
			t.Fatalf("tinygo release args should keep scheduler explicit-only: %v", args)
		}
	}
}

func TestTinyGoBrowserReleaseDebugInfoKeepsDWARF(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoProfileEnv, TinyGoProfileFast)
	t.Setenv(TinyGoDebugInfoEnv, "true")

	// Resolve the compiler arguments for the configured TinyGo policy.
	platform := parseTestPlatform(t, "web/js/wasm")
	args, err := newTinyGoBuildArgs(platform, bldr_manifest.BuildType_RELEASE, "spacewave-core.wasm", []string{"build_type_release", "purego"})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the excluded TinyGo compiler arguments.
	if slices.Contains(args, "-no-debug") {
		t.Fatalf("tinygo debug args should keep debug info: %v", args)
	}
	if slices.Contains(args, tinyGoInternalNoDWARFArg) {
		t.Fatalf("tinygo debug args should keep DWARF: %v", args)
	}
}

func TestTinyGoDebugInfoRejectsUnknownValue(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoDebugInfoEnv, "sometimes")

	// Resolve the compiler arguments for the configured TinyGo policy.
	platform := parseTestPlatform(t, "web/js/wasm")
	_, err := newTinyGoBuildArgs(platform, bldr_manifest.BuildType_RELEASE, "spacewave-core.wasm", []string{"build_type_release", "purego"})
	if err == nil {
		t.Fatal("expected invalid TinyGo debug info env to fail")
	}

	// Verify that the compiler error identifies the invalid setting.
	if !strings.Contains(err.Error(), TinyGoDebugInfoEnv) {
		t.Fatalf("error = %q, want %s", err.Error(), TinyGoDebugInfoEnv)
	}
}

func TestTinyGoTraceWorkArgsAreExplicit(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoTraceEnv, "true")
	t.Setenv(TinyGoWorkEnv, "true")

	// Resolve the compiler arguments for the configured TinyGo policy.
	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}

	// Verify every required TinyGo compiler argument.
	for _, want := range []string{"-x", "-work"} {

		// Verify the required TinyGo compiler arguments.
		if !slices.Contains(args, want) {
			t.Fatalf("tinygo args = %v, want %s", args, want)
		}
	}
}

func TestTinyGoTraceRejectsUnknownValue(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoTraceEnv, "sometimes")

	// Resolve the compiler arguments for the configured TinyGo policy.
	_, err := GetDefaultTinygoArgs()
	if err == nil {
		t.Fatal("expected invalid TinyGo trace env to fail")
	}

	// Verify that the compiler error identifies the invalid setting.
	if !strings.Contains(err.Error(), TinyGoTraceEnv) {
		t.Fatalf("error = %q, want %s", err.Error(), TinyGoTraceEnv)
	}
}

func TestTinyGoWorkRejectsUnknownValue(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoWorkEnv, "sometimes")

	// Resolve the compiler arguments for the configured TinyGo policy.
	_, err := GetDefaultTinygoArgs()
	if err == nil {
		t.Fatal("expected invalid TinyGo work env to fail")
	}

	// Verify that the compiler error identifies the invalid setting.
	if !strings.Contains(err.Error(), TinyGoWorkEnv) {
		t.Fatalf("error = %q, want %s", err.Error(), TinyGoWorkEnv)
	}
}

func TestGoWasmOptimizeEnabledDefaultsOn(t *testing.T) {
	t.Setenv(GoWasmOptimizeEnv, "")

	enabled, err := GoWasmOptimizeEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("expected Go wasm optimization to default on")
	}
}

func TestGoWasmOptimizeEnabledCanBeDisabled(t *testing.T) {
	for _, value := range []string{"0", "false", "no", "off"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(GoWasmOptimizeEnv, value)

			enabled, err := GoWasmOptimizeEnabled()
			if err != nil {
				t.Fatal(err)
			}
			if enabled {
				t.Fatalf("%s=%s should disable Go wasm optimization", GoWasmOptimizeEnv, value)
			}
		})
	}
}

func TestGoWasmOptimizeRejectsUnknownValue(t *testing.T) {
	t.Setenv(GoWasmOptimizeEnv, "sometimes")

	_, err := GoWasmOptimizeEnabled()
	if err == nil {
		t.Fatal("expected invalid Go wasm optimize env to fail")
	}
	if !strings.Contains(err.Error(), GoWasmOptimizeEnv) {
		t.Fatalf("error = %q, want %s", err.Error(), GoWasmOptimizeEnv)
	}
}

func TestTinyGoBrowserDevArgsDoNotUseInternalNoDWARF(t *testing.T) {
	// Configure an invalid Go wasm optimization setting.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoProfileEnv, TinyGoProfileFast)

	// Resolve the invalid Go wasm optimization setting.
	platform := parseTestPlatform(t, "web/js/wasm")
	args, err := newTinyGoBuildArgs(platform, bldr_manifest.BuildType_DEV, "spacewave-core.wasm", []string{"build_type_dev", "purego"})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the required TinyGo compiler arguments.
	if !slices.Contains(args, "-no-debug") {
		t.Fatalf("tinygo web dev args = %v, want existing non-native -no-debug behavior", args)
	}

	// Verify the excluded TinyGo compiler arguments.
	if slices.Contains(args, tinyGoInternalNoDWARFArg) {
		t.Fatalf("tinygo web dev args should not use release-only %s: %v", tinyGoInternalNoDWARFArg, args)
	}
}

func TestTinyGoNonBrowserReleaseArgsDoNotUseInternalNoDWARF(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoProfileEnv, TinyGoProfileFast)

	// Resolve the compiler arguments for the configured TinyGo policy.
	platform := parseTestPlatform(t, "web/wasi/wasm")
	args, err := newTinyGoBuildArgs(platform, bldr_manifest.BuildType_RELEASE, "spacewave-core.wasm", []string{"build_type_release", "purego"})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the required TinyGo compiler arguments.
	if !slices.Contains(args, "-no-debug") {
		t.Fatalf("tinygo wasi release args = %v, want release -no-debug behavior", args)
	}

	// Verify the excluded TinyGo compiler arguments.
	if slices.Contains(args, tinyGoInternalNoDWARFArg) {
		t.Fatalf("tinygo wasi release args should not use browser-only %s: %v", tinyGoInternalNoDWARFArg, args)
	}
	if slices.Contains(args, "-tags=build_type_release purego "+BldrTinyGoJSImportBuildTag) {
		t.Fatalf("tinygo wasi release args should not use browser-only JS imports tag: %v", args)
	}
}

func TestDefaultTinyGoArgsPrintsPanicWhenConfigured(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoPanicStrategyEnv, "print")

	// Resolve the compiler arguments for the configured TinyGo policy.
	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the required TinyGo compiler arguments.
	if !slices.Contains(args, "-panic=print") {
		t.Fatalf("tinygo args = %v, want -panic=print", args)
	}
}

func TestDefaultTinyGoArgsTrapPanicWhenConfigured(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoPanicStrategyEnv, "trap")

	// Resolve the compiler arguments for the configured TinyGo policy.
	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the required TinyGo compiler arguments.
	if !slices.Contains(args, "-panic=trap") {
		t.Fatalf("tinygo args = %v, want -panic=trap", args)
	}
}

func TestDefaultTinyGoArgsRejectsUnknownPanicStrategy(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoPanicStrategyEnv, "bogus")

	// Resolve the compiler arguments for the configured TinyGo policy.
	_, err := GetDefaultTinygoArgs()
	if err == nil {
		t.Fatal("expected invalid TinyGo panic strategy to fail")
	}

	// Verify that the compiler error identifies the invalid setting.
	if !strings.Contains(err.Error(), TinyGoPanicStrategyEnv) {
		t.Fatalf("error = %q, want %s", err.Error(), TinyGoPanicStrategyEnv)
	}
}

func TestTinyGoArgsUseExplicitIdentityInputs(t *testing.T) {
	// Configure the TinyGo environment for this compiler argument case.
	clearTinyGoOptionEnv(t)
	t.Setenv(TinyGoProfileEnv, TinyGoProfileFast)
	t.Setenv(TinyGoOptEnv, "1")
	t.Setenv(TinyGoGCEnv, "leaking")
	t.Setenv(TinyGoLLVMFeaturesEnv, "+atomics,+bulk-memory")

	// Resolve the compiler arguments for the configured TinyGo policy.
	args, err := GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}

	// Verify every required TinyGo compiler argument.
	for _, want := range []string{
		"-opt=1",
		"-gc=leaking",
		"-llvm-features=+atomics,+bulk-memory",
		"-stack-size=" + TinyGoDefaultStackSize,
		"-interp-timeout=10m",
	} {

		// Verify the required TinyGo compiler arguments.
		if !slices.Contains(args, want) {
			t.Fatalf("tinygo args = %v, want %s", args, want)
		}
	}
}

func clearTinyGoOptionEnv(t *testing.T) {
	t.Helper()
	for _, key := range TinyGoStartupCacheEnvKeys() {
		t.Setenv(key, "")
	}
}

func parseTestPlatform(t *testing.T, id string) bldr_platform.Platform {
	t.Helper()
	platform, err := bldr_platform.ParsePlatform(id)
	if err != nil {
		t.Fatal(err)
	}
	return platform
}
