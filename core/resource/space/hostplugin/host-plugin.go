package hostplugin

import (
	"context"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// Resolve returns the stored host plugin ID or the plugin context fallback.
func Resolve(ctx context.Context, stored string) string {
	if stored != "" {
		return stored
	}
	if info := bldr_plugin.GetPluginContextInfo(ctx); info != nil {
		return info.GetPluginMeta().GetPluginId()
	}
	return ""
}
