//go:build !js

package bldr_web_bundler

import web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"

// WebPkgResolveConfigs converts WebPkgRefConfig values to the cycle-free resolve
// config used after a bundler has discovered package roots.
func WebPkgResolveConfigs(configs []*WebPkgRefConfig) []web_pkg.WebPkgResolveConfig {
	out := make([]web_pkg.WebPkgResolveConfig, len(configs))
	for i, conf := range configs {
		var entrypoints []web_pkg.WebPkgEntrypointConfig
		for _, entrypoint := range conf.GetEntrypoints() {
			entrypoints = append(entrypoints, web_pkg.WebPkgEntrypointConfig{Path: entrypoint.GetPath()})
		}
		out[i] = web_pkg.WebPkgResolveConfig{
			ID:          conf.GetId(),
			Exclude:     conf.GetExclude(),
			Entrypoints: entrypoints,
		}
	}
	return out
}
