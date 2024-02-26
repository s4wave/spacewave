//go:build deps_only
// +build deps_only

package devtool

// Import the necessary entrypoints for the devtool bundle.
import (
	// _ imports the browser entrypoint
	_ "github.com/aperturerobotics/bldr/devtool/web/entrypoint"
	// _ imports the browser entrypoint controller
	_ "github.com/aperturerobotics/bldr/devtool/web/entrypoint/controller"
	// _ imports the browser init msgs
	_ "github.com/aperturerobotics/bldr/devtool/web"
)
