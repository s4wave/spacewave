package bldr_plugin_builder

import (
	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/hydra/bucket"
)

// PluginBuilderResult is the output of a plugin builder.
type PluginBuilderResult struct {
	// PluginManifest is the plugin manifest object.
	PluginManifest *plugin.PluginManifest
	// PluginManifestRef is the plugin manifest object ref.
	PluginManifestRef *bucket.ObjectRef
}

// NewPluginBuilderResult builds the result object.
func NewPluginBuilderResult(manifest *plugin.PluginManifest, ref *bucket.ObjectRef) *PluginBuilderResult {
	return &PluginBuilderResult{
		PluginManifest:    manifest,
		PluginManifestRef: ref,
	}
}
