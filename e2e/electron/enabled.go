//go:build !js

package electron

import (
	"os"
	"strings"
)

// E2EElectronEnabled reports whether the heavy Electron e2e suite should run.
func E2EElectronEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ENABLE_E2E_ELECTRON")), "true")
}
