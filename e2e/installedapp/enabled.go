//go:build !skip_e2e && !js

package installedapp

import (
	"os"
	"strings"
)

// E2EInstalledAppEnabled reports whether the heavy installed-app e2e suite
// should run.
func E2EInstalledAppEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ENABLE_E2E_INSTALLED_APP")), "true")
}
