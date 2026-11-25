package plugin_host_scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/routine"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
)

// TestWatchWorldManifestWakesOnLinkOfStoredManifest checks the running World
// watch selects a manifest whose object was already stored when a later store
// only links it to the plugin host. That store leaves the host's revision in
// place, so the watch must wake on the edge it read. The held case is the
// first selection after a relaunch: FetchManifest went idle and announced the
// manifest before its store linked it.
func TestWatchWorldManifestWakesOnLinkOfStoredManifest(t *testing.T) {
	for _, held := range []bool{false, true} {
		name := "selected"
		if held {
			name = "awaiting-fetch"
		}
		t.Run(name, func(t *testing.T) {
			testWatchWorldManifestWakesOnLinkOfStoredManifest(t, held)
		})
	}
}

func testWatchWorldManifestWakesOnLinkOfStoredManifest(t *testing.T, held bool) {
	// Bound the test and open a World testbed with a bus engine.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	ws := world.NewEngineWorldState(tb.Engine, true)

	// Create the plugin host and store a manifest object it does not link yet.
	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err)
	}
	var manifestRef *bldr_manifest.ManifestRef
	var manifestKey string
	if err := world.ExecTransaction(ctx, tb.Engine, true, func(ctx context.Context, tx world.WorldState) error {
		manifestRef, manifestKey = storeTestWorldManifest(t, ctx, tx, "spacewave-core", "desktop/darwin/arm64", 1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Run the World manifest watch for the plugin.
	le := tb.Logger
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	hostSet := &pluginHostSet{pluginHosts: []bldr_plugin_host.PluginHost{host}}
	pi := &pluginInstance{
		c: &Controller{
			bus:    tb.Bus,
			conf:   &Config{EngineId: tb.EngineID},
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	if held {
		// FetchManifest went idle having announced the manifest.
		pi.c.conf.AwaitFetchManifest = true
		pi.c.conf.WatchFetchManifest = true
		pi.announcedManifests.Store(&[]string{manifestKey})
	}
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	go func() {
		_ = pi.execWatchWorldManifest(watchCtx, hostSet)
	}()

	// Wait for the first selection, which finds no linked manifest. A held
	// selection records nothing, so give it time to reach its wait.
	if held {
		time.Sleep(200 * time.Millisecond)
	}
	for !held && pi.manifestSelectionFingerprint.Load() == nil {
		if ctx.Err() != nil {
			t.Fatal("watch never completed its first selection")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pi.executePluginRoutine.GetState() != nil {
		t.Fatal("expected no selection before the manifest is linked")
	}

	// Store the fetched manifest. The manifest object already holds it, so the
	// store writes only the link from the plugin host.
	manifestRev := func() uint64 {
		obj, err := world.MustGetObject(ctx, ws, manifestKey)
		if err != nil {
			t.Fatal(err)
		}
		defer world.ReleaseObjectState(obj)
		_, rev, err := obj.GetRootRef(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return rev
	}
	before := manifestRev()
	if err := bldr_manifest_world.ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{objKey}, manifestRef); err != nil {
		t.Fatal(err)
	}
	if after := manifestRev(); after != before {
		t.Fatalf("manifest object revision moved from %d to %d; the case needs a link-only store", before, after)
	}

	// The watch must select the newly linked manifest.
	deadline := time.Now().Add(3 * time.Second)
	for {
		state := pi.executePluginRoutine.GetState()
		if state != nil && state.manifestSnapshot.GetManifestRef().EqualVT(manifestRef.GetManifestRef()) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("watch did not select the manifest linked to the plugin host")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
