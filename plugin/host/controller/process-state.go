package plugin_host_controller

import (
	"context"

	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/sirupsen/logrus"
)

// ProcessState implements the state reconciliation loop.
func (c *Controller) ProcessState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	objKey := c.objKey
	if obj == nil {
		le.Debug("object does not exist, waiting")
		return true, nil
	}

	pluginPlatformID, err := c.hostPluginPlatformID.Await(ctx)
	if err != nil {
		return true, err
	}

	// collect all connected plugin manifests
	collManifests, manifestErrs, err := bldr_manifest_world.CollectManifests(ctx, ws, pluginPlatformID, objKey)
	if err != nil {
		le.WithError(err).Warn("unable to list plugin host manifests")
		return true, err
	}
	for _, err := range manifestErrs {
		le.WithError(err).Warn("skipped invalid manifest")
	}

	// use the latest version of the manifests
	var execManifestKeys []string
	for _, manifestList := range collManifests {
		// the list is sorted by revision, newer is earlier.
		execManifestKeys = append(execManifestKeys, manifestList[0].ManifestKey)
	}
	c.syncWatchPluginManifests(execManifestKeys)

	return true, nil
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = ((*Controller)(nil)).ProcessState
