package bldr_dist_compiler

import (
	"fmt"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

const embeddedVolumeFmt = `package %s

import (
	"embed"
	"io/fs"

	plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// DistFS contains the plugin distribution files.
//
//go:embed dist
var DistFS embed.FS

// AssetsFS contains the plugin assets.
//
//go:embed assets
var AssetsFS embed.FS

// PluginMeta is the plugin metadata encoded in b58.
var PluginMeta = %q

// Entrypoint is the plugin entrypoint.
var Entrypoint = %q

// GetDistFS returns DistFS with the dist/ prefix stripped.
func GetDistFS() fs.FS {
	f, _ := fs.Sub(DistFS, "dist")
	return f
}

// GetAssetsFS returns AssetsFS with the assets/ prefix stripped.
func GetAssetsFS() fs.FS {
	f, _ := fs.Sub(AssetsFS, "assets")
	return f
}

// StaticPlugin is the static plugin definition.
var StaticPlugin = plugin.NewStaticPlugin(
	plugin.NewPluginManifest(
		plugin.MustUnmarshalPluginManifestB58(PluginMeta),
		Entrypoint,
	),
	GetDistFS(),
	GetAssetsFS(),
)
`

// FormatEmbeddedVolumeFile formats the embedded kvfile code.
func FormatEmbeddedVolumeFile(
	packageName string,
	meta *bldr_manifest.ManifestMeta,
	entrypoint string,
) string {
	return fmt.Sprintf(embeddedVolumeFmt, packageName, meta.MarshalB58(), entrypoint)
}
