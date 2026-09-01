package gocompiler

import (
	"slices"
	"strings"
	"testing"
)

// TestDefaultNonNativeBuildStripsWasmDebugSymbols verifies the existing
// default: every non-native (and release) Go build passes -w -s.
func TestDefaultNonNativeBuildStripsWasmDebugSymbols(t *testing.T) {
	if !shouldDropWasmDebugSymbols(false, false, true, false) {
		t.Fatal("default non-native dev wasm build should drop debug symbols")
	}
	if !shouldDropWasmDebugSymbols(true, true, false, false) {
		t.Fatal("default native release build should drop debug symbols")
	}
	if shouldDropWasmDebugSymbols(false, true, false, false) {
		t.Fatal("default native dev build should keep debug symbols")
	}
}

// TestDiagnosticWasmBuildKeepsDebugSymbols verifies the diagnostic-only mode
// keeps the linker name section for WASM output only.
func TestDiagnosticWasmBuildKeepsDebugSymbols(t *testing.T) {
	if shouldDropWasmDebugSymbols(true, false, true, true) {
		t.Fatal("diagnostic non-native release wasm build must keep debug symbols")
	}
	if shouldDropWasmDebugSymbols(false, false, true, true) {
		t.Fatal("diagnostic non-native dev wasm build must keep debug symbols")
	}
}

// TestDiagnosticFlagCannotUnstripNativeRelease verifies the diagnostic flag
// cannot unstrip native release output when inherited outside the profiler.
func TestDiagnosticFlagCannotUnstripNativeRelease(t *testing.T) {
	if !shouldDropWasmDebugSymbols(true, true, false, true) {
		t.Fatal("native release build must still strip with diagnostic mode")
	}
	if !shouldDropWasmDebugSymbols(true, true, true, true) {
		t.Fatal("native release output must still strip even if isWasmOutput true")
	}
}

// TestDiagnosticWasmModeBypassesWasmOptPostProcessing verifies the actual
// release wasm-opt gating decision: diagnostic mode skips wasm-opt, default
// keeps it, and BLDR_GO_WASM_OPTIMIZE=false also skips it.
func TestDiagnosticWasmModeBypassesWasmOptPostProcessing(t *testing.T) {
	t.Setenv(GoWasmOptimizeEnv, "")
	optimize, err := shouldRunWasmOpt(false)
	if err != nil {
		t.Fatal(err)
	}
	if !optimize {
		t.Fatal("default release wasm build should run wasm-opt")
	}

	optimize, err = shouldRunWasmOpt(true)
	if err != nil {
		t.Fatal(err)
	}
	if optimize {
		t.Fatal("diagnostic release wasm build must skip wasm-opt")
	}

	t.Setenv(GoWasmOptimizeEnv, "false")
	optimize, err = shouldRunWasmOpt(false)
	if err != nil {
		t.Fatal(err)
	}
	if optimize {
		t.Fatal("BLDR_GO_WASM_OPTIMIZE=false should skip wasm-opt")
	}
}

// TestGoWasmDiagnosticEnabledParsesTypedValues verifies the diagnostic env
// parser mirrors GoWasmOptimize's accepted true and false values.
func TestGoWasmDiagnosticEnabledParsesTypedValues(t *testing.T) {
	for _, raw := range []string{"", "0", "false", "no", "off"} {
		t.Setenv(GoWasmDiagnosticEnv, raw)
		enabled, err := GoWasmDiagnosticEnabled()
		if err != nil {
			t.Fatalf("%s=%q: unexpected error: %s", GoWasmDiagnosticEnv, raw, err.Error())
		}
		if enabled {
			t.Fatalf("%s=%q should disable diagnostic mode", GoWasmDiagnosticEnv, raw)
		}
	}
	for _, raw := range []string{"1", "true", "yes", "on"} {
		t.Setenv(GoWasmDiagnosticEnv, raw)
		enabled, err := GoWasmDiagnosticEnabled()
		if err != nil {
			t.Fatalf("%s=%q: unexpected error: %s", GoWasmDiagnosticEnv, raw, err.Error())
		}
		if !enabled {
			t.Fatalf("%s=%q should enable diagnostic mode", GoWasmDiagnosticEnv, raw)
		}
	}
}

// TestGoWasmDiagnosticEnabledRejectsUnknownValue verifies malformed values are
// an error so a typo can never silently produce a stripped diagnostic profile.
func TestGoWasmDiagnosticEnabledRejectsUnknownValue(t *testing.T) {
	t.Setenv(GoWasmDiagnosticEnv, "sometimes")
	_, err := GoWasmDiagnosticEnabled()
	if err == nil {
		t.Fatal("expected invalid diagnostic value to fail")
	}
	if !strings.Contains(err.Error(), GoWasmDiagnosticEnv) {
		t.Fatalf("error = %q, want %s", err.Error(), GoWasmDiagnosticEnv)
	}
}

// TestDiagnosticWasmModeParticipatesInCacheIdentity verifies the diagnostic
// env key participates in startup cache identity inputs.
func TestDiagnosticWasmModeParticipatesInCacheIdentity(t *testing.T) {
	keys := GoWasmDiagnosticStartupCacheEnvKeys()
	if !slices.Contains(keys, GoWasmDiagnosticEnv) {
		t.Fatalf("cache identity keys = %v, want %s", keys, GoWasmDiagnosticEnv)
	}
}
