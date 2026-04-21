package bldr_plugin

import (
	"context"
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
func NewPluginContextInfo(meta *PluginMeta) *PluginContextInfo {
	return &PluginContextInfo{
		PluginMeta: meta,
	}
}

// Validate validates the context info.
func (i *PluginContextInfo) Validate() error {
	if err := i.GetPluginMeta().Validate(); err != nil {
		return err
	}
	return nil
}
