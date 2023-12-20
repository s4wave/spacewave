package plugin_entrypoint_context

import (
	"context"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
)

var pluginContextInfoKey = &struct{ pluginContextInfoKey string }{}

// WithPluginContextInfo attaches plugin information to a context.
func WithPluginContextInfo(ctx context.Context, info *PluginContextInfo) context.Context {
	return context.WithValue(ctx, pluginContextInfoKey, info)
}

// GetPluginContextInfo retrieves plugin information from a context.
// May return nil.
func GetPluginContextInfo(ctx context.Context, info *PluginContextInfo) *PluginContextInfo {
	result := ctx.Value(pluginContextInfoKey)
	info, ok := result.(*PluginContextInfo)
	if !ok {
		return nil
	}
	return info
}

// NewPluginContextInfo constructs a new PluginContextInfo object.
func NewPluginContextInfo(meta *bldr_plugin.PluginMeta) *PluginContextInfo {
	return &PluginContextInfo{
		PluginMeta: meta,
	}
}
