//go:build !goscript

package provider_spacewave_handoff

import (
	"os/exec"
	"runtime"

	"github.com/pkg/errors"
)

// openBrowser opens the default browser to the given URL without shell
// interpretation. The URL must already be validated by validateOpenURL.
func openBrowser(rawURL string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "--", rawURL).Start()
	case "linux":
		return exec.Command("xdg-open", rawURL).Start()
	case "windows":
		// rundll32 passes rawURL to url.dll's FileProtocolHandler as a single
		// argument; it does not shell-interpret the value. Avoid cmd /c start,
		// which treats the URL as command line input.
		return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", rawURL).Start()
	default:
		return errors.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
