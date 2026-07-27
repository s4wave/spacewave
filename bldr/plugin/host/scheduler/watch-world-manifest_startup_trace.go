//go:build bldr_startup_trace && !tinygo

package plugin_host_scheduler

import (
	"context"

	startuptrace "github.com/s4wave/spacewave/db/traceutil/startup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// processManifestWorldState processes the state for the PluginManifest.
func (t *pluginInstance) processManifestWorldState(
	ctx context.Context,
	le *logrus.Entry,
	hosts *pluginHostSet,
	ws world.WorldState,
	obj world.ObjectState,
) (waitForChanges bool, err error) {
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/plugin-host-scheduler/eligibility-collect")
	outcome := "error"
	defer func() {
		startuptrace.Log(traceCtx, "outcome", outcome)
		task.End()
	}()
	waitForChanges, err = t.processManifestWorldStateCore(traceCtx, le, hosts, ws, obj)
	if err == nil {
		outcome = "ok"
	}
	return waitForChanges, err
}
