package plugin_static

import (
	"context"
	"io/fs"

	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/timestamp"
)

// StaticPlugin is the initial version of a plugin to be loaded on startup.
// The contents of the plugin distribution files are passed as an io/fs.
type StaticPlugin struct {
	// Manifest is the plugin manifest, excluding the DistFs and AssetFs fields.
	Manifest *plugin.PluginManifest
	// PluginDistFs is the filesystem to copy to distfs.
	PluginDistFs fs.FS
	// PluginAssetsFs is the filesystem to copy to assetfs.
	PluginAssetsFs fs.FS
}

// NewStaticPlugin constructs a new StaticPlugin.
func NewStaticPlugin(manifest *plugin.PluginManifest, pluginDistFs, pluginAssetsFs fs.FS) *StaticPlugin {
	return &StaticPlugin{
		Manifest:       manifest,
		PluginDistFs:   pluginDistFs,
		PluginAssetsFs: pluginAssetsFs,
	}
}

// CreatePluginManifest creates the plugin manifest from the static plugin.
func (p *StaticPlugin) CreatePluginManifest(ctx context.Context, bcs *block.Cursor, ts *timestamp.Timestamp) error {
	return plugin.CreatePluginManifest(
		ctx,
		bcs,
		p.Manifest.PluginId,
		p.Manifest.Entrypoint,
		p.PluginDistFs,
		p.PluginAssetsFs,
		plugin.BuildType(p.Manifest.BuildType),
		ts,
	)
}
