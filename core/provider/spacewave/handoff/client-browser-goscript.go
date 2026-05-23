//go:build goscript

package provider_spacewave_handoff

import (
	"runtime"

	"github.com/pkg/errors"
)

// openBrowser returns the native unsupported-platform result without importing
// os/exec into the GoScript package graph.
func openBrowser(string) error {
	return errors.Errorf("unsupported platform: %s", runtime.GOOS)
}
