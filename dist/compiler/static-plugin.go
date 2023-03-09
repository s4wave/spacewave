package dist_compiler

import (
	"fmt"
	"strings"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
)

// LabelToPackageName converts a plugin or app ID to a package name.
func LabelToPackageName(kind, pluginID string) string {
	packageName := strings.TrimSpace(pluginID)
	packageName = strings.ReplaceAll(packageName, "-", "_")
	for strings.Contains(packageName, "__") {
		packageName = strings.ReplaceAll(packageName, "__", "_")
	}
	return kind + "_" + packageName
}

const staticPluginFmt = `package %s

import (
	"embed"
	"io/fs"

	plugin "github.com/aperturerobotics/bldr/plugin"
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

// FormatStaticPluginFile formats static-plugin.go.
func FormatStaticPluginFile(
	packageName string,
	meta *bldr_plugin.PluginManifestMeta,
	entrypoint string,
) string {
	return fmt.Sprintf(staticPluginFmt, packageName, meta.MarshalB58(), entrypoint)
}
