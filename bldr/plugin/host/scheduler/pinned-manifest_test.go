package plugin_host_scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// TestPinnedManifestRetainsExactRevision exercises immutable selection through stored World manifests.
func TestPinnedManifestRetainsExactRevision(t *testing.T) {
	// Store two executable revisions under the same plugin host.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)
	ws, err := world_block.BuildMockWorldState(ctx, le, true, cursor, false)
	if err != nil {
		t.Fatal(err)
	}
	const hostKey = "plugin-host"
	if _, err := manifest_world.CreateManifestStore(ctx, ws, hostKey); err != nil {
		t.Fatal(err)
	}
	old, _ := storeTestWorldManifest(t, ctx, ws, "colors", "js", 1)
	newer, _ := storeTestWorldManifest(t, ctx, ws, "colors", "js", 2)
	oldKey := manifest.NewManifestKey(hostKey, old.GetMeta())
	for _, ref := range []*manifest.ManifestRef{old, newer} {
		key := manifest.NewManifestKey(hostKey, ref.GetMeta())
		if err := manifest_world.ExStoreManifestOp(ctx, ws, "", key, []string{hostKey}, ref); err != nil {
			t.Fatal(err)
		}
	}
	// An unrelated retained revision lives in a closed Space. Exact lookup must
	// filter its identity without opening that unavailable bucket.
	closed, _ := storeTestWorldManifest(t, ctx, ws, "colors", "js", 3)
	closed.GetManifestRef().BucketId = "closed-space"
	if err := manifest_world.ExStoreManifestOp(ctx, ws, "", "a-closed-artifact", []string{hostKey}, closed); err != nil {
		t.Fatal(err)
	}
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{&testPluginHost{id: "js"}}}
	instance := &pluginInstance{
		c: &Controller{conf: &Config{WatchFetchManifest: true, DisableStoreManifest: true}, objKey: hostKey}, le: le, pluginID: "colors",
		manifestRoot:            old.GetManifestRef().GetRootRef().GetHash().MarshalString(),
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
		pluginLoadStateCtr:      ccontainer.NewCContainer[bldr_plugin.PluginLoadState](bldr_plugin.NewPluginLoadState(nil, bldr_plugin.InitialCapabilityRegistrationPending)),
		runningPluginCtr:        ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
	}
	selectManifest := func() {
		t.Helper()
		obj, found, err := ws.GetObject(ctx, hostKey)
		defer world.ReleaseObjectState(obj)
		if err != nil || !found {
			t.Fatalf("host object: %t, %v", found, err)
		}
		if _, err := instance.processManifestWorldState(ctx, le, hosts, ws, obj); err != nil {
			t.Fatal(err)
		}
	}

	// A newer installed revision does not alter the pinned execution.
	selectManifest()
	selected := instance.executePluginRoutine.GetState()
	if selected == nil || !selected.manifestSnapshot.GetManifestRef().EqualVT(old.GetManifestRef()) {
		t.Fatalf("pinned selection = %#v, want %s", selected, old.GetManifestRef().String())
	}

	// Losing the exact artifact is unavailable, even while the newer one remains.
	if err := ws.DeleteGraphQuad(ctx, manifest_world.NewManifestQuad(hostKey, oldKey, "colors")); err != nil {
		t.Fatal(err)
	}
	selectManifest()
	if instance.executePluginRoutine.GetState() != nil {
		t.Fatal("missing pinned artifact fell back to another executable")
	}

	// Restoring the artifact makes the same pinned request usable without new code.
	if err := ws.SetGraphQuad(ctx, manifest_world.NewManifestQuad(hostKey, oldKey, "colors")); err != nil {
		t.Fatal(err)
	}
	selectManifest()
	selected = instance.executePluginRoutine.GetState()
	if selected == nil || !selected.manifestSnapshot.GetManifestRef().EqualVT(old.GetManifestRef()) {
		t.Fatal("restored pinned artifact was not selected")
	}
}
