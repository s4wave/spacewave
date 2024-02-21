//go:build deps_only
// +build deps_only

package devtool_entrypoint

// Import the necessary entrypoints for the devtool bundle.
import (
	// _ imports the browser entrypoint
	_ "github.com/aperturerobotics/bldr/devtool/entrypoint/browser"
)
