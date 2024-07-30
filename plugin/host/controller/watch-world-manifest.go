package plugin_host_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/sirupsen/logrus"
)

// execute executes the tracker.
func (t *executePlugin) execWatchWorldManifest(ctx context.Context) error {
	t.le.Debugf("starting watch world manifests")
	objLoop := world_control.NewWatchLoop(
		t.le.WithField("object-loop", "watch-world-manifest"),
		t.c.objKey,
		t.processManifestWorldState,
	)
	return world_control.ExecuteBusWatchLoop(
		ctx,
		t.c.bus,
		t.c.conf.GetEngineId(),
		false,
		objLoop,
	)
}

// processManifestWorldState processes the state for the PluginManifest.
func (t *executePlugin) processManifestWorldState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	_ *bucket.ObjectRef, _ uint64,
) (waitForChanges bool, err error) {
	if obj == nil {
		t.le.Warnf("plugin host not found: %v", t.c.objKey)
		return true, nil
	}

	// determine host plugin platform id
	hostPluginPlatformID, err := t.c.hostPluginPlatformID.Await(ctx)
	if err != nil {
		return true, err
	}

	// Lookup the latest PluginManifest matching our plugin linked to PluginHost.
	manifests, _, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		ws,
		t.pluginID,
		hostPluginPlatformID,
		t.c.objKey,
	)
	if err != nil {
		return true, err
	}
	if len(manifests) == 0 {
		t.le.Infof("no manifests for plugin found in world")
		return true, nil
	}

	// select the manifest with highest revision
	manifest := manifests[0]

	// create the snapshot
	manifestSnapshot := &bldr_manifest.ManifestSnapshot{
		ManifestRef: manifest.ManifestRef,
		Manifest:    manifest.Manifest,
	}

	// update the state container (which automatically diffs the manifest and restarts if changed)
	manifest.Manifest.GetMeta().Logger(t.le).Info("got latest manifest from world")
	t.executePluginRoutine.SetState(manifestSnapshot)

	return true, nil
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = ((*executePlugin)(nil)).processManifestWorldState
