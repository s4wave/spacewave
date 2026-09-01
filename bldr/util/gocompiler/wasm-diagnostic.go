package gocompiler

import (
	"os"
	"strings"

	"github.com/pkg/errors"
)

const (
	// GoWasmDiagnosticEnv enables the diagnostic-only unstripped Go WASM build
	// mode: the Go linker keeps its name section (no -w -s) and release
	// post-processing (wasm-opt debug stripping) is bypassed. Output is for
	// CPU-profile symbolization only and is never used for release artifacts.
	GoWasmDiagnosticEnv = "BLDR_GO_WASM_DIAGNOSTIC"
)

// GoWasmDiagnosticStartupCacheEnvKeys returns env keys that affect the
// diagnostic wasm artifact identity (same cache-key space as the normal build).
func GoWasmDiagnosticStartupCacheEnvKeys() []string {
	return []string{GoWasmDiagnosticEnv}
}

// GoWasmDiagnosticEnabled reports whether the diagnostic unstripped wasm mode
// is requested via env. Unsupported values are an error, mirroring
// GoWasmOptimizeEnabled, so a typo can never silently produce a stripped
// profile while the diagnostic mode appears requested.
func GoWasmDiagnosticEnabled() (bool, error) {
	return goWasmDiagnosticEnabled(os.Getenv(GoWasmDiagnosticEnv))
}

func goWasmDiagnosticEnabled(raw string) (bool, error) {
	switch strings.TrimSpace(raw) {
	case "", "0", "false", "no", "off":
		return false, nil
	case "1", "true", "yes", "on":
		return true, nil
	default:
		return false, errors.Errorf("unsupported %s value %q, expected true or false", GoWasmDiagnosticEnv, raw)
	}
}
