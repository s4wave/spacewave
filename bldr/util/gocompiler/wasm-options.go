package gocompiler

import (
	"os"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

const (
	// GoWasmOptimizeEnv controls the Binaryen wasm-opt pass after release Go
	// WebAssembly builds. It defaults on for release artifacts.
	GoWasmOptimizeEnv = "BLDR_GO_WASM_OPTIMIZE"
)

var goWasmOptimizeStartupCacheEnvKeys = []string{
	GoWasmOptimizeEnv,
}

// GoWasmOptimizeStartupCacheEnvKeys returns env keys that affect Go wasm artifact identity.
func GoWasmOptimizeStartupCacheEnvKeys() []string {
	return slices.Clone(goWasmOptimizeStartupCacheEnvKeys)
}

// GoWasmOptimizeEnabled reports whether release Go wasm builds should run wasm-opt.
func GoWasmOptimizeEnabled() (bool, error) {
	return goWasmOptimizeEnabled(os.Getenv(GoWasmOptimizeEnv))
}

func goWasmOptimizeEnabled(raw string) (bool, error) {
	switch strings.TrimSpace(raw) {
	case "", "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, errors.Errorf("unsupported %s value %q, expected true or false", GoWasmOptimizeEnv, raw)
	}
}
