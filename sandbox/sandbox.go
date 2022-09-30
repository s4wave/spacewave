package sandbox

import (
	"embed"
	"io/fs"

	"github.com/aperturerobotics/bldr/entrypoint"
	"github.com/aperturerobotics/bldr/plugin"
	plugin_static "github.com/aperturerobotics/bldr/plugin/static"
)

//go:embed dist
var pluginDistFs embed.FS

// TODO: use the Bldr CLI instead of hardcoding this.
func init() {
	// open the dist directory
	distDir, err := fs.Sub(pluginDistFs, "dist")
	if err != nil {
		panic(err)
	}
	entrypoint.RootPlugin = &plugin_static.StaticPlugin{
		Manifest: &plugin.PluginManifest{
			PluginId:   "sandbox",
			Entrypoint: "plugin-main.go",
		},
		PluginDistFs: distDir,
	}
}
