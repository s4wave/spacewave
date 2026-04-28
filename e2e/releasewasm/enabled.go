//go:build !js

package releasewasm

import (
	"os"
	"strings"
)

// E2EReleaseWasmEnabled reports whether the heavy release WASM suite should run.
func E2EReleaseWasmEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ENABLE_E2E_RELEASE_WASM")), "true")
}
