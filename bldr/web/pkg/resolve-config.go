//go:build !js

package web_pkg

// WebPkgResolveConfig holds the config needed to resolve a web package.
// Avoids importing web/bundler proto types to prevent import cycles.
type WebPkgResolveConfig struct {
	// ID identifies the package to resolve.
	ID string
	// Exclude omits this package from resolution.
	Exclude bool
	// Entrypoints selects the subpaths the package serves.
	Entrypoints []WebPkgEntrypointConfig
}
