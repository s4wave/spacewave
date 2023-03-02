//go:build deps_only
// +build deps_only

package dist

// Import all Go packages which are referenced by the dist entrypoints.
import (
	// _ imports logrus
	_ "github.com/sirupsen/logrus"
	// _ imports dist/entrypoint
	_ "github.com/aperturerobotics/bldr/dist/entrypoint"
)
