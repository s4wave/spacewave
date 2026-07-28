//go:build !bldr_startup_trace || tinygo

package manifest_fetch_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// reconcileManifests re-collects and emits the manifests for this directive.
func (r *fetchManifestResolver) reconcileManifests(
	ctx context.Context,
	le *logrus.Entry,
	handler directive.ResolverHandler,
	ws world.WorldState,
) (bool, error) {
	return r.reconcileManifestsCore(ctx, le, handler, ws)
}
