package bldr_plugin

import (
	"context"
	"strings"
)

var pluginContextInfoKey = &struct{ pluginContextInfoKey string }{}

// WithPluginContextInfo attaches plugin information to a context.
func WithPluginContextInfo(ctx context.Context, info *PluginContextInfo) context.Context {
	return context.WithValue(ctx, pluginContextInfoKey, info)
}

// GetPluginContextInfo retrieves plugin information from a context.
// May return nil.
func GetPluginContextInfo(ctx context.Context) *PluginContextInfo {
	result := ctx.Value(pluginContextInfoKey)
	info, ok := result.(*PluginContextInfo)
	if !ok {
		return nil
	}
	return info
}

// NewPluginContextInfo constructs a new PluginContextInfo object.
func NewPluginContextInfo(meta *PluginMeta, instanceKey, manifestRoot string) *PluginContextInfo {
	return &PluginContextInfo{
		PluginMeta:   meta,
		InstanceKey:  instanceKey,
		ManifestRoot: manifestRoot,
	}
}

// PinAssetsHref pins href, a path under the plugin's assets route, to the
// manifest root the plugin runs, so a browser loads the files of that exact
// build. A Space serves its plugins' files only at pinned paths. It returns
// href unchanged when the root is unknown or href selects other files.
func (i *PluginContextInfo) PinAssetsHref(href string) string {
	// Cut the plugin's assets route from href.
	pluginID := i.GetPluginMeta().GetPluginId()
	file, ok := strings.CutPrefix(href, PluginAssetsHttpPrefix+pluginID+"/")
	root := i.GetManifestRoot()
	if !ok || root == "" {
		return href
	}

	// Insert the manifest root after the plugin ID.
	return PluginAssetsHttpPrefix + PluginArtifactID(pluginID, root) + "/" + file
}
