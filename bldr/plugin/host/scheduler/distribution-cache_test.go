package plugin_host_scheduler

import (
	"testing"

	"github.com/aperturerobotics/util/routine"
	dist "github.com/s4wave/spacewave/bldr/dist"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// TestDistributionCacheExcludesPreviousInstall checks real World selection with
// a higher-revision plugin retained from an earlier distribution.
func TestDistributionCacheExcludesPreviousInstall(t *testing.T) {
	// Store both installations in one application World, as on a shared state root.
	ctx := t.Context()
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
	old, oldKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "desktop/darwin/arm64", 14)
	current, currentKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "desktop/darwin/arm64", 1)
	oldMeta := &dist.DistMeta{ChannelKey: "stable", DistWorldRef: old.GetManifestRef()}
	currentMeta := &dist.DistMeta{ChannelKey: "staging", DistWorldRef: current.GetManifestRef()}
	oldHost, err := dist.PluginHostObjectKey(oldMeta)
	if err != nil {
		t.Fatal(err)
	}
	hostKey, err := dist.PluginHostObjectKey(currentMeta)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"plugin-host", oldHost, hostKey} {
		if _, err := manifest_world.CreateManifestStore(ctx, ws, key); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"plugin-host", oldHost} {
		if err := ws.SetGraphQuad(ctx, manifest_world.NewManifestQuad(key, oldKey, "spacewave-core")); err != nil {
			t.Fatal(err)
		}
	}

	// The new host waits for its own source, even while the old cache is usable.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{host}}
	instance := &pluginInstance{
		c: &Controller{conf: &Config{}, objKey: hostKey}, le: le, pluginID: "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	selectManifest := func() *executePluginArgs {
		t.Helper()
		obj, found, err := ws.GetObject(ctx, hostKey)
		if obj != nil {
			defer world.ReleaseObjectState(obj)
		}
		if err != nil || !found {
			t.Fatalf("host object: found=%t, error=%v", found, err)
		}
		if _, err := instance.processManifestWorldState(ctx, le, hosts, ws, obj); err != nil {
			t.Fatal(err)
		}
		return instance.executePluginRoutine.GetState()
	}
	if selected := selectManifest(); selected != nil {
		t.Fatal("previous installation ran before this distribution's source arrived")
	}

	// This build's lower artifact revision wins and remains available on a warm reopen.
	if err := ws.SetGraphQuad(ctx, manifest_world.NewManifestQuad(hostKey, currentKey, "spacewave-core")); err != nil {
		t.Fatal(err)
	}
	selected := selectManifest()
	if selected == nil || !selected.manifestSnapshot.GetManifestRef().EqualVT(current.GetManifestRef()) {
		t.Fatalf("selection = %v, want current distribution", selected)
	}
	reopenedKey, err := dist.PluginHostObjectKey(currentMeta.CloneVT())
	if err != nil || reopenedKey != hostKey {
		t.Fatalf("reopened cache = %q, error=%v", reopenedKey, err)
	}
	if _, err := dist.ParsePluginHostObjectKey(reopenedKey); err != nil {
		t.Fatal(err)
	}
}
