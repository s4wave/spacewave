package dist_compiler

import (
	"fmt"
	"strings"
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
	"github.com/aperturerobotics/bldr/plugin"
)

// DistFS contains the plugin distribution files.
//
//go:embed dist
var DistFS embed.FS

// AssetsFS contains the plugin assets.
//
//go:embed assets
var AssetsFS embed.FS

// PluginID is the plugin identifier.
var PluginID = "%s"

// Entrypoint is the path to the plugin entrypoint.
var Entrypoint = "%s"

// BuildType is the build type used to compile the plugin.
var BuildType = "%s"

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
		PluginID,
		Entrypoint,
		plugin.ToBuildType(BuildType),
	),
	GetDistFS(),
	GetAssetsFS(),
)
`

// FormatStaticPluginFile formats static-plugin.go.
func FormatStaticPluginFile(
	packageName, pluginID, entrypoint, buildType string,
) string {
	return fmt.Sprintf(staticPluginFmt, packageName, pluginID, entrypoint, buildType)
}
