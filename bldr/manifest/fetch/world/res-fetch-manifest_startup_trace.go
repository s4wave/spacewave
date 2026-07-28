//go:build bldr_startup_trace && !tinygo

package manifest_fetch_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	startuptrace "github.com/s4wave/spacewave/db/traceutil/startup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// reconcileManifests re-collects and emits the manifests for this directive.
//
// The task covers one reconciliation rather than the resolver as a whole. The
// resolver blocks in its watch loop until the directive is released, so a task
// scoped to it stays open past the end of the startup capture and contributes
// no duration.
func (r *fetchManifestResolver) reconcileManifests(
	ctx context.Context,
	le *logrus.Entry,
	handler directive.ResolverHandler,
	ws world.WorldState,
) (bool, error) {
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/manifest-fetch-world/resolve")
	defer task.End()
	startuptrace.Log(traceCtx, "manifest-id", r.dir.GetManifestId())
	waitForChanges, err := r.reconcileManifestsCore(traceCtx, le, handler, ws)
	if err != nil {
		startuptrace.Log(traceCtx, "outcome", "error")
		return waitForChanges, err
	}
	startuptrace.Log(traceCtx, "outcome", "ok")
	return waitForChanges, nil
}
