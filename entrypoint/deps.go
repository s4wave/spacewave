//go:build deps_only
// +build deps_only

package bldr_web

// Import the necessary entrypoints for the dist bundle.
import (
	// _ imports the browser entrypoint
	_ "github.com/aperturerobotics/bldr/entrypoint/browser"
)
