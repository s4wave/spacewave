package plugin_host_scheduler

import (
	"context"
	"crypto/rand"
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
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// TestPinnedManifestRetainsExactRevision exercises immutable selection through stored World manifests.
func TestPinnedManifestRetainsExactRevision(t *testing.T) {
	// Store two executable revisions under the same plugin host.
	// Bind a bounded context and logger for the test.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Open a World testbed and an empty cursor over it.
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

	// Build a mock World state over the cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, cursor, false)
	if err != nil {
		t.Fatal(err)
	}
	const hostKey = "plugin-host"

	// Create the plugin-host manifest store and store both executable revisions.
	if _, err := manifest_world.CreateManifestStore(ctx, ws, hostKey); err != nil {
		t.Fatal(err)
	}
	old, _ := storeTestWorldManifest(t, ctx, ws, "colors", "js", 1)
	newer, _ := storeTestWorldManifest(t, ctx, ws, "colors", "js", 2)
	oldKey := manifest.NewManifestKey(hostKey, old.GetMeta())

	// A signed release stores an authorization envelope whose own root differs from the executable.
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.SignReleaseAuthorization(key); err != nil {
		t.Fatal(err)
	}

	// Store both revisions under their manifest keys.
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

	// Build the plugin instance pinned to the old revision's root.
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{&testPluginHost{id: "js"}}}
	instance := &pluginInstance{
		c: &Controller{conf: &Config{WatchFetchManifest: true, DisableStoreManifest: true}, objKey: hostKey}, le: le, pluginID: "colors",
		manifestRoot:            old.GetManifestRef().GetRootRef().GetHash().MarshalString(),
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
		pluginLoadStateCtr:      ccontainer.NewCContainer[bldr_plugin.PluginLoadState](bldr_plugin.NewPluginLoadState(nil, bldr_plugin.InitialCapabilityRegistrationPending)),
		runningPluginCtr:        ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
	}

	// selectManifest runs manifest selection over the stored host object.
	selectManifest := func() {
		// Load the host object, releasing it before returning.
		t.Helper()
		obj, found, err := ws.GetObject(ctx, hostKey)
		defer world.ReleaseObjectState(obj)
		if err != nil || !found {
			t.Fatalf("host object: %t, %v", found, err)
		}

		// Run selection over the loaded host object.
		if _, _, err := instance.processManifestWorldState(ctx, le, hosts, ws, obj); err != nil {
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

	// An untyped direct manifest remains usable beside an unreadable reference envelope.
	const legacyKey = "legacy-direct"
	legacy, err := ws.CreateObject(ctx, legacyKey, old.GetManifestRef())
	world.ReleaseObjectState(legacy)
	if err != nil {
		t.Fatal(err)
	}

	// Keep a sibling whose envelope block is missing, so it cannot supply a root match.
	missing := old.GetManifestRef().Clone()
	missing.RootRef.Hash.Hash[0] ^= 0xff
	broken, err := ws.CreateObject(ctx, "broken-envelope", missing)
	world.ReleaseObjectState(broken)
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, ws, "broken-envelope", manifest_world.ManifestRefTypeID); err != nil {
		t.Fatal(err)
	}

	// Replace the signed graph edge with its legacy equivalent without repairing the broken sibling.
	if err := ws.DeleteGraphQuad(ctx, manifest_world.NewManifestQuad(hostKey, oldKey, "colors")); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{legacyKey, "broken-envelope"} {
		if err := ws.SetGraphQuad(ctx, manifest_world.NewManifestQuad(hostKey, key, "colors")); err != nil {
			t.Fatal(err)
		}
	}
	selectManifest()
	selected = instance.executePluginRoutine.GetState()
	if selected == nil || !selected.manifestSnapshot.GetManifestRef().EqualVT(old.GetManifestRef()) {
		t.Fatal("legacy pinned artifact was blocked by an unreadable sibling")
	}
}
