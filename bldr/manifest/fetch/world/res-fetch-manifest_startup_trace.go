//go:build bldr_startup_trace && !tinygo

package manifest_fetch_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	startuptrace "github.com/s4wave/spacewave/db/traceutil/startup"
)

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	traceCtx, task := startuptrace.NewTask(ctx, "bldr/manifest-fetch-world/resolve")
	defer task.End()
	return r.resolve(traceCtx, handler)
}
