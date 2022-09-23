package plugin_static

import (
	"io/fs"

	"github.com/aperturerobotics/bldr/plugin"
)

// StaticPlugin is the initial version of a plugin to be loaded on startup.
// The contents of the plugin distribution files are passed as an io/fs.
type StaticPlugin struct {
	// Manifest is the plugin manifest, excluding the DistFs field.
	Manifest *plugin.PluginManifest
	// PluginDistFs is the filesystem to copy to distfs.
	PluginDistFs fs.FS
}

// NewStaticPlugin constructs a new StaticPlugin.
func NewStaticPlugin(manifest *plugin.PluginManifest, pluginDistFs fs.FS) *StaticPlugin {
	return &StaticPlugin{
		Manifest:     manifest,
		PluginDistFs: pluginDistFs,
	}
}
