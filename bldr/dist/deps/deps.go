//go:build deps_only
// +build deps_only

package dist_deps

// Import all Go packages which are referenced by the dist entrypoints.
import (
	// _ imports logrus
	_ "github.com/sirupsen/logrus"
	// _ imports dist/entrypoint
	_ "github.com/s4wave/spacewave/bldr/dist/entrypoint"
)
