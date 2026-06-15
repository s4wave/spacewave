package bldr_web_bundler

import web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"

// GetBldrDistWebPkgRefConfigs returns the web pkg ref configs for BldrExternal.
//
// Each config carries its web-pkg-root-relative entry imports so consumer
// bundles can remap imports to the same served names buildWebPkg emits, rather
// than re-deriving them from the package's on-disk layout (whose dist/ subdir
// and .pb.js filenames differ from the served names).
func GetBldrDistWebPkgRefConfigs() []*WebPkgRefConfig {
	configs := make([]*WebPkgRefConfig, len(web_pkg_external.BldrExternal))
	for i, externalPkg := range web_pkg_external.BldrExternal {
		configs[i] = &WebPkgRefConfig{
			Id:      externalPkg,
			Imports: web_pkg_external.BldrDistWebPkgImports[externalPkg],
		}
	}
	return configs
}
