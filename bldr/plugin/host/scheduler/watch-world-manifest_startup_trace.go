//go:build bldr_startup_trace && !tinygo

package plugin_host_scheduler

import (
	"context"

	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	startuptrace "github.com/s4wave/spacewave/db/traceutil/startup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// processManifestWorldState traces processManifestWorldStateCore.
func (t *pluginInstance) processManifestWorldState(
	ctx context.Context,
	le *logrus.Entry,
	hosts *pluginHostSet,
	ws world.WorldState,
	obj world.ObjectState,
) (waitForChanges bool, missing []*bldr_manifest_world.StartupManifestCandidateEligibility, err error) {
	// Record the selection outcome on the startup trace task.
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/plugin-host-scheduler/eligibility-collect")
	outcome := "error"
	defer func() {
		startuptrace.Log(traceCtx, "outcome", outcome)
		task.End()
	}()

	// Process the World state under the traced context.
	waitForChanges, missing, err = t.processManifestWorldStateCore(traceCtx, le, hosts, ws, obj)
	if err == nil {
		outcome = "ok"
	}
	return waitForChanges, missing, err
}
