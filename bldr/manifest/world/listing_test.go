package bldr_manifest_world

import (
	"context"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestManifestListingWaitChange checks that manifest listings wait past unrelated
// graph writes and rerun for changes to the manifest and type edges they read.
func TestManifestListingWaitChange(t *testing.T) {
	// Exercise each listing with additions and removals of relevant edges.
	tests := []struct {
		// name describes the listing and changed edge.
		name string
		// list is the production listing interface under test.
		list func(context.Context, world.WorldState, ...string) ([]string, error)
		// edge is the relevant quad to add or remove.
		edge world.GraphQuad
		// remove reports whether the relevant edge starts present.
		remove bool
	}{
		{"manifest/add-link", ListManifests, NewManifestQuad("listing/bundle", "listing/next", "app"), false},
		{"manifest/remove-link", ListManifests, NewManifestQuad("listing/bundle", "listing/next", "app"), true},
		{"manifest/add-type", ListManifests, world.NewGraphQuadWithKeys("listing/candidate", world_types.TypePred.String(), world_types.BuildTypeObjectKey(ManifestTypeID), ""), false},
		{"manifest/remove-type", ListManifests, world.NewGraphQuadWithKeys("listing/candidate", world_types.TypePred.String(), world_types.BuildTypeObjectKey(ManifestRefTypeID), ""), true},
		{"candidate/add-link", ListManifestCandidates, NewManifestQuad("listing/bundle", "listing/next", "app"), false},
		{"candidate/remove-link", ListManifestCandidates, NewManifestQuad("listing/bundle", "listing/next", "app"), true},
		{"empty/add-link", ListManifestCandidates, NewManifestQuad("listing/empty", "listing/next", "app"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build a real World with an intermediate bundle and an untyped candidate.
			ctx := t.Context()
			tb := world_testbed.MustDefault(t, ctx)
			ws := tb.WorldState
			for _, key := range []string{"listing/store", "listing/bundle", "listing/candidate", "listing/next", "listing/unrelated", "listing/empty", world_types.BuildTypeObjectKey(ManifestTypeID), world_types.BuildTypeObjectKey(ManifestRefTypeID)} {
				obj, err := ws.CreateObject(ctx, key, nil)
				world.ReleaseObjectState(obj)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, edge := range []world.GraphQuad{
				NewManifestQuad("listing/store", "listing/bundle", ""),
				NewManifestQuad("listing/bundle", "listing/candidate", "app"),
				world.NewGraphQuadWithKeys("listing/next", world_types.TypePred.String(), world_types.BuildTypeObjectKey(ManifestTypeID), ""),
			} {
				if err := ws.SetGraphQuad(ctx, edge); err != nil {
					t.Fatal(err)
				}
			}
			if tt.remove {
				if err := ws.SetGraphQuad(ctx, tt.edge); err != nil {
					t.Fatal(err)
				}
			}

			// Record the listing's reads before any subsequent graph writes.
			startKey := "listing/store"
			if tt.name == "empty/add-link" {
				startKey = "listing/empty"
			}
			seqno, err := ws.GetSeqno(ctx)
			if err != nil {
				t.Fatal(err)
			}
			reads := world.NewReadSet()
			before, err := tt.list(ctx, reads.Track(ws), startKey)
			if err != nil {
				t.Fatal(err)
			}

			// Change unrelated manifest and type edges, and another predicate on a read node.
			for _, edge := range []world.GraphQuad{
				NewManifestQuad("listing/unrelated", "listing/next", "app"),
				world.NewGraphQuadWithKeys("listing/unrelated", world_types.TypePred.String(), world_types.BuildTypeObjectKey(ManifestTypeID), ""),
				world.NewGraphQuadWithKeys("listing/bundle", "<other>", "listing/next", ""),
			} {
				if err := ws.SetGraphQuad(ctx, edge); err != nil {
					t.Fatal(err)
				}
			}

			// Read the real change feed before a relevant write can join this report.
			waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			err = world.WatchChanges(waitCtx, ws, seqno, func(_ uint64, changes *world.ChangeSet) error {
				if reads.Touched(changes) {
					t.Error("unrelated World changes would rerun the listing")
				}
				return io.EOF
			})
			if err != io.EOF {
				t.Fatalf("read unrelated changes: %v", err)
			}

			// Change a relevant edge and record exactly which commit should wake the view.
			if tt.remove {
				err = ws.DeleteGraphQuad(ctx, tt.edge)
			} else {
				err = ws.SetGraphQuad(ctx, tt.edge)
			}
			if err != nil {
				t.Fatal(err)
			}
			want, err := ws.GetSeqno(ctx)
			if err != nil {
				t.Fatal(err)
			}

			// Use the same read-set wait as production watches, with a bounded deadline.
			got, err := reads.WaitChange(waitCtx, ws, seqno)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("listing woke at seqno %d, want relevant edge commit %d", got, want)
			}

			// Rerun the listing and check that the relevant edge changed its result.
			after, err := tt.list(ctx, ws, startKey)
			if err != nil {
				t.Fatal(err)
			}
			if slices.Equal(before, after) {
				t.Fatalf("listing stayed %v after relevant edge change", after)
			}
		})
	}
}

// TestManifestListingTraversal checks that tracked listings retain the Cayley
// path's depth limit, cycle handling, type filtering and sorted, distinct output.
func TestManifestListingTraversal(t *testing.T) {
	// Seed a wide graph with a cycle, converging edges and a path beyond fifty hops.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	err := world.ExecTransaction(ctx, tb.Engine, true, func(ctx context.Context, ws world.WorldState) error {
		for i := range 140 {
			key := "traversal/" + strconv.Itoa(i)
			obj, err := ws.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
			if err := world_types.SetObjectType(ctx, ws, key, ManifestTypeID); err != nil {
				return err
			}
		}
		for i := 1; i < 140; i++ {
			src := 0
			if i < 55 {
				src = i - 1
			}
			if err := ws.SetGraphQuad(ctx, NewManifestQuad("traversal/"+strconv.Itoa(src), "traversal/"+strconv.Itoa(i), "app")); err != nil {
				return err
			}
		}
		for _, edge := range []world.GraphQuad{
			NewManifestQuad("traversal/1", "traversal/0", ""),
			NewManifestQuad("traversal/55", "traversal/2", ""),
			NewManifestQuad("traversal/0", "traversal/55", "other"),
		} {
			if err := ws.SetGraphQuad(ctx, edge); err != nil {
				return err
			}
		}
		return world_types.SetObjectType(ctx, ws, "traversal/3", ManifestBundleTypeID)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Compare both production listings with their existing Cayley path contracts.
	for _, tt := range []struct {
		// list collects a production listing.
		list func(context.Context, world.WorldState, ...string) ([]string, error)
		// path selects the equivalent Cayley traversal.
		path func(*cayley.Path) *cayley.Path
	}{
		{ListManifestCandidates, NewListManifestCandidatePath},
		{ListManifests, NewListManifestPath},
	} {
		want, err := world.CollectPathWithKeys(ctx, tb.WorldState, []string{"traversal/0", "traversal/0"}, func(p *cayley.Path) (*cayley.Path, error) {
			return tt.path(p), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err := tt.list(ctx, tb.WorldState, "traversal/0", "traversal/0")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("listing = %v, want path results %v", got, want)
		}
	}
}
func TestDumpStartupManifestGraphForManifestIDIncludesRetainedRefDiagnostics(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newManifestTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a readable good manifest ref and link its graph edge.
	goodRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, goodRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a bad manifest ref pointing at a missing bucket.
	badRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9)
	badRef.GetManifestRef().BucketId = "missing-bucket"
	const badRefKey = "plugin-host/ref/missing-bucket"
	storeTestManifestRefObject(t, ctx, ws, badRefKey, badRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, badRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a legacy manifest ref with an empty graph label.
	legacyRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 11)
	legacyRef.GetManifestRef().BucketId = "legacy-missing-bucket"
	const legacyRefKey = "plugin-host/ref/legacy-missing-bucket"
	storeTestManifestRefObject(t, ctx, ws, legacyRefKey, legacyRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, legacyRefKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	// Dump the startup manifest graph for the manifest ID.
	dump, err := DumpStartupManifestGraphForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Assert the dump contains the expected graph diagnostics.
	for _, want := range []string{
		"startup manifest graph manifest_id=spacewave-web platform_ids=js",
		"root plugin-host type=bldr/manifest-store",
		"edge plugin-host -> plugin-host/ref/good label=<spacewave-web>",
		"edge plugin-host -> plugin-host/ref/missing-bucket label=<spacewave-web>",
		"edge plugin-host -> plugin-host/ref/legacy-missing-bucket label=<empty>",
		"candidate plugin-host/ref/good type=<unknown>",
		"ref_meta=manifest_id=spacewave-web,build_type=production,platform_id=js,rev=7",
		"manifest_meta=manifest_id=spacewave-web,build_type=production,platform_id=js,rev=7",
		"candidate plugin-host/ref/missing-bucket type=<unknown>",
		"manifest_bucket=missing-bucket",
		"manifest_bucket=legacy-missing-bucket",
		"skip=bucket not found",
	} {
		if !strings.Contains(dump, want) {
			t.Fatalf("dump missing %q:\n%s", want, dump)
		}
	}
	if !strings.Contains(dump, "manifest_root="+badRef.GetManifestRef().GetRootRef().MarshalString()) {
		t.Fatalf("dump missing bad manifest root ref:\n%s", dump)
	}
	if !strings.Contains(dump, "object_root=") {
		t.Fatalf("dump missing object root refs:\n%s", dump)
	}
}

func TestDumpStartupManifestGraphDirectCandidateStaysLocalOnly(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Add a lookup observer controller that records network lookups.
	lookupObserver := &startupManifestGraphLookupObserver{called: make(chan struct{}, 1)}
	observerRel, err := tb.Bus.AddController(ctx, lookupObserver, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer observerRel()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Configure a lookup-directive bucket for the diagnostic dump.
	const lookupBucketID = "startup-graph-diagnostic-bucket"
	bucketLkConfig, err := bucket.NewLookupConfig(configset.NewControllerConfig(1, &lookup_concurrent.Config{
		NotFoundBehavior: lookup_concurrent.NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE,
	}))
	if err != nil {
		t.Fatal(err.Error())
	}
	bucketConf, err := bucket.NewConfig(lookupBucketID, 1, bucketLkConfig)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, bucketConf); err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}
	directRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 13)
	directRef.GetManifestRef().BucketId = lookupBucketID
	const directKey = "plugin-host/direct/external"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), directKey, directRef.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, directKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Dump the startup manifest graph under a short context deadline.
	dumpCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	dump, err := DumpStartupManifestGraphForManifestID(
		dumpCtx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !strings.Contains(dump, "candidate "+directKey) {
		t.Fatalf("dump missing direct candidate:\n%s", dump)
	}
	select {
	case <-lookupObserver.called:
		t.Fatalf("diagnostic graph dump invoked LookupBlockFromNetwork:\n%s", dump)
	default:
	}
}

func TestDumpStartupManifestGraphForManifestIDClassifiesProvenance(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newManifestTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a global release manifest candidate.
	releaseRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 10)
	const releaseKey = "release/manifests/spacewave-web/js"
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), releaseKey, []string{storeKey}, releaseRef); err != nil {
		t.Fatal(err.Error())
	}

	// Store a project build manifest candidate with a build-result marker.
	buildRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9)
	const buildKey = "project/build/spacewave-web/js"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), buildKey, buildRef.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := createStartupGraphBuildResultMarker(ctx, ws, buildKey); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, buildKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a space-local manifest candidate.
	spaceLocalRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 8)
	const spaceLocalKey = "spaces/test-space/plugins/generated/manifest"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), spaceLocalKey, spaceLocalRef.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, spaceLocalKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store an unknown-provenance manifest candidate.
	unknownRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const unknownKey = "plugin-host/ref/unknown"
	storeTestManifestRefObject(t, ctx, ws, unknownKey, unknownRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, unknownKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Dump the startup manifest graph for the manifest ID.
	dump, err := DumpStartupManifestGraphForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Assert each candidate carries its expected provenance classification.
	assertStartupGraphDumpLine(t, dump, "candidate "+releaseKey, "provenance=global-release", "derived=true", "protected=false")
	assertStartupGraphDumpLine(t, dump, "candidate "+buildKey, "provenance=project-build", "derived=true", "protected=false")
	assertStartupGraphDumpLine(t, dump, "candidate "+spaceLocalKey, "provenance=space-local-or-ephemeral", "derived=false", "protected=true")
	assertStartupGraphDumpLine(t, dump, "candidate "+unknownKey, "provenance=unknown", "derived=false", "protected=true")
}

func createStartupGraphBuildResultMarker(ctx context.Context, ws world.WorldState, manifestKey string) error {

	// Write the build-result marker manifest as a World object.
	ref, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs.SetBlock(manifest.NewManifest(manifest.NewManifestMeta("build-result-marker", manifest.BuildType_DEV, "js", 1), "entrypoint"), true)
		return nil
	})
	if err != nil {
		return err
	}
	objKey := manifestKey + "/build-result"
	{
		createdObject, err := ws.CreateObject(ctx, objKey, ref)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			return err
		}
	}
	return world_types.SetObjectType(ctx, ws, objKey, "bldr/manifest-build-result")
}

func assertStartupGraphDumpLine(t *testing.T, dump string, prefix string, parts ...string) {
	t.Helper()
	for line := range strings.SplitSeq(dump, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		for _, part := range parts {
			if !strings.Contains(line, part) {
				t.Fatalf("dump line %q missing %q", line, part)
			}
		}
		return
	}
	t.Fatalf("dump missing line prefix %q:\n%s", prefix, dump)
}
