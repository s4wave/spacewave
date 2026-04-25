//go:build !js

package wasm

import (
	"os"
	"strings"
)

// E2EWasmEnabled reports whether the heavy e2e/wasm suites should run.
func E2EWasmEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ENABLE_E2E_WASM")), "true")
}
