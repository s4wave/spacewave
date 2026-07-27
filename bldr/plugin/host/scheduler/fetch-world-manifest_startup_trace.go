//go:build bldr_startup_trace && !tinygo

package plugin_host_scheduler

import (
	"context"

	startuptrace "github.com/s4wave/spacewave/db/traceutil/startup"
)

// execFetchWorldManifest executes the world manifest fetch tracker.
func (t *pluginInstance) execFetchWorldManifest(ctx context.Context, hosts *pluginHostSet) error {
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/plugin-host-scheduler/fetch-world-manifest")
	defer task.End()
	return t.execFetchWorldManifestCore(traceCtx, hosts)
}

// execFetchManifestValueStorer executes storing the FetchManifest value in storage.
func (t *fetchManifestValueStorer) execFetchManifestValueStorer(ctx context.Context) error {
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/plugin-host-scheduler/store-manifest")
	defer task.End()
	return t.execFetchManifestValueStorerCore(traceCtx)
}
