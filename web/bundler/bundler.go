package bldr_web_bundler

import web_pkg_external "github.com/aperturerobotics/bldr/web/pkg/external"

// GetBldrDistWebPkgRefConfigs returns the web pkg ref configs for BldrExternal.
func GetBldrDistWebPkgRefConfigs() []*WebPkgRefConfig {
	configs := make([]*WebPkgRefConfig, len(web_pkg_external.BldrExternal))
	for i, externalPkg := range web_pkg_external.BldrExternal {
		configs[i] = &WebPkgRefConfig{Id: externalPkg}
	}
	return configs
}
