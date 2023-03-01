//go:build deps_only
// +build deps_only

package dist

// Import all Go packages which are referenced by the dist entrypoint.
import (
	// _ imports dist/entrypoint
	dist_entrypoint "github.com/aperturerobotics/bldr/dist/entrypoint"
	// _ imports bldr/plugin
	plugin "github.com/aperturerobotics/bldr/plugin"
	// _ imports logrus
	_ "github.com/sirupsen/logrus"
)
