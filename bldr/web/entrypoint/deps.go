//go:build deps_only
// +build deps_only

package bldr_entrypoint

// Import the necessary entrypoints for the dist bundle.
import (
	// _ imports the browser entrypoint
	_ "github.com/s4wave/spacewave/bldr/web/entrypoint/browser"
	// _ imports bldr_values
	_ "github.com/s4wave/spacewave/bldr/values"
)
