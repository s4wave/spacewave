//go:build !bldr_startup_trace || tinygo

package plugin_host_scheduler

import (
	"context"

	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
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
) (bool, []*bldr_manifest_world.StartupManifestCandidateEligibility, error) {
	return t.processManifestWorldStateCore(ctx, le, hosts, ws, obj)
}
