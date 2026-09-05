package bldr_manifest_world

import (
	"context"
	"testing"
	"time"

	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	"github.com/s4wave/spacewave/db/testbed"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestStartupEligibilitySkipsUnavailableDirectManifest(t *testing.T) {
	// Build the real World and observe forbidden network fallback.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVolumeConfig(&volume_kvtxinmem.Config{
		VolumeConfig: &volume_controller.Config{
			VolumeIdAlias: []string{bldr_dist.StaticBlockStoreID},
		},
	}))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Detect network recovery attempts for missing embedded blocks.
	lookupObserver := &startupManifestGraphLookupObserver{called: make(chan struct{}, 1)}
	observerRel, err := tb.Bus.AddController(ctx, lookupObserver, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(observerRel)

	// Store plugin references in the local World.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(ocs.Release)

	// Use the same World object graph as the startup scheduler.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Model an embedded bucket whose previous binary is no longer installed.
	bucketConf, err := bldr_dist.NewDistBucketConfig("test")
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, bucketConf); err != nil {
		t.Fatal(err.Error())
	}

	// Retain the direct manifest reference as an earlier startup would.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}
	directRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 13)
	directRef.GetManifestRef().BucketId = bucketConf.GetId()
	directRef.GetManifestRef().RootRef.Hash.Hash[0] ^= 0xff
	const directKey = "plugin-host/direct/external"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), directKey, directRef.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, directKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// The current distribution supplies a readable replacement.
	currentRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 14)
	const currentKey = "plugin-host/direct/current"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), currentKey, currentRef.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, currentKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Classify the missing candidate without waiting for network recovery.
	collectCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	t.Cleanup(cancel)
	candidates, err := CollectStartupManifestEligibilityForManifestID(
		collectCtx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// A stale direct reference must not prevent selecting an available revision.
	stale := findStartupCandidateByKey(t, candidates, directKey)
	if stale.Eligibility != StartupManifestEligibilityUnsafe {
		t.Fatalf("stale eligibility = %s: %s", stale.Eligibility, stale.Reason)
	}
	selectable := SelectableStartupManifests(candidates)
	if len(selectable) != 1 || selectable[0].GetRev() != 14 {
		t.Fatalf("selectable manifests = %v, want only current revision 14", selectable)
	}
	select {
	case <-lookupObserver.called:
		t.Fatal("eligibility lookup invoked the network for a missing retained manifest")
	default:
	}
}
