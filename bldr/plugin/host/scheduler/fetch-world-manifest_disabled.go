//go:build !bldr_startup_trace || tinygo

package plugin_host_scheduler

import "context"

// execFetchWorldManifest executes the world manifest fetch tracker.
func (t *pluginInstance) execFetchWorldManifest(ctx context.Context, hosts *pluginHostSet) error {
	return t.execFetchWorldManifestCore(ctx, hosts)
}

// execFetchManifestValueStorer executes storing the FetchManifest value in storage.
func (t *fetchManifestValueStorer) execFetchManifestValueStorer(ctx context.Context) error {
	return t.execFetchManifestValueStorerCore(ctx)
}
