package plugin_host_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// pluginManifestTracker tracks matched PluginManifest objects.
// updates the in-memory cache & restarts plugin if the manifest changes.
type pluginManifestTracker struct {
	// c is the controller
	c *Controller
	// objKey is the object key
	objKey string
	// objLoop tracks the object changes
	objLoop *world_control.WatchLoop
	// prevObjRev is the previous object rev.
	prevObjRev uint64
	// prevObjRef is the previous object ref.
	prevObjRef *bucket.ObjectRef
}

// newPluginManifestTracker constructs a new plugin manifest tracker routine.
func (c *Controller) newPluginManifestTracker(key string) (keyed.Routine, *pluginManifestTracker) {
	tr := &pluginManifestTracker{
		c:      c,
		objKey: key,
	}
	tr.objLoop = world_control.NewWatchLoop(
		c.le.WithField("object-loop", "plugin-manifest-tracker"),
		key,
		tr.processState,
	)
	return tr.execute, tr
}

// execute executes the tracker.
func (t *pluginManifestTracker) execute(ctx context.Context) error {
	objKey, le := t.objKey, t.c.le

	le.Debugf("starting plugin manifest tracker: %s", objKey)
	return world_control.ExecuteBusWatchLoop(
		ctx,
		t.c.bus,
		t.c.conf.GetEngineId(),
		false,
		t.objLoop,
	)
}

// processState processes the state for the PluginManifest.
func (t *pluginManifestTracker) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	// fetch the PluginManifest from the rootRef.
	var pluginManifest *bldr_manifest.Manifest
	var pluginManifestRef *bucket.ObjectRef
	err = ws.AccessWorldState(ctx, rootRef, func(bls *bucket_lookup.Cursor) error {
		_, bcs := bls.BuildTransaction(nil)
		var err error
		pluginManifestRef = bls.GetRefWithOpArgs()
		pluginManifest, err = bldr_manifest.UnmarshalManifest(bcs)
		return err
	})
	if err != nil {
		return true, err
	}

	// not found
	pluginID := pluginManifest.GetMeta().GetManifestId()
	if pluginID == "" {
		le.
			WithField("plugin-manifest-key", t.objKey).
			WithField("plugin-manifest-ref", rootRef.MarshalString()).
			Debug("plugin manifest not found or empty")
		return true, nil
	}
	if err := pluginManifest.Validate(); err != nil {
		return true, errors.Wrap(err, "invalid plugin manifest")
	}

	// if the object rev & ref did not change, ignore
	if rev == t.prevObjRev || rootRef.EqualsRef(t.prevObjRef) {
		return true, nil
	}
	t.prevObjRev, t.prevObjRef = rev, rootRef

	// update the manifest in the set
	t.c.rmtx.Lock()
	existing := t.c.pluginManifests[pluginID]
	changed := !pluginManifest.EqualVT(existing.manifest)
	if changed {
		le.Infof("plugin manifest updated: %s at %d", t.objKey, rev)
		t.c.pluginManifests[pluginID] = pluginManifestSnapshot{
			objKey:      t.objKey,
			manifest:    pluginManifest,
			manifestRef: pluginManifestRef,
		}

		// restart the plugin, if running
		if _, reset := t.c.pluginInstances.RestartRoutine(pluginID); reset {
			le.Info("restarted outdated plugin instance")
		}
	}
	t.c.rmtx.Unlock()

	return true, nil
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = ((*pluginManifestTracker)(nil)).processState
