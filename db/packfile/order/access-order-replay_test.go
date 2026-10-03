package order

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

func TestReplayAccessOrderRecordPrioritizesProfileThenFallback(t *testing.T) {
	// Create references for the access profile and fallback candidates.
	ctx := context.Background()
	profiledFirst := testRef(t, "profiled-first")
	profiledSecond := testRef(t, "profiled-second")
	fallbackRoot := testRef(t, "fallback-root")
	fallbackChild := testRef(t, "fallback-child")
	staleRef := testRef(t, "stale-ref")

	// Connect the fallback object root to its child block.
	graph := newTestRefGraph()
	graph.add(block_gc.ObjectIRI("fallback-object"), block_gc.BlockIRI(fallbackRoot))
	graph.add(block_gc.BlockIRI(fallbackRoot), block_gc.BlockIRI(fallbackChild))

	// Capture the profiled paths in their recorded access order.
	record := testAccessOrderRecord(t, []*AccessOrderEntry{
		{
			Filesystem: AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST,
			Path:       "entrypoint.mjs",
		},
		{
			Filesystem: AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST,
			Path:       "missing.mjs",
		},
		{
			Filesystem: AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST,
			Path:       "chunk.mjs",
		},
	})

	// Resolve profile paths with shared, missing, and stale block references.
	resolver := AccessOrderPathResolverFunc(func(_ context.Context, filesystem AccessOrderFilesystem, fpath string) ([]*block.BlockRef, bool, error) {
		// Require profile lookups to use the distribution filesystem.
		if filesystem != AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST {
			t.Fatalf("filesystem = %s, want dist", filesystem)
		}

		// Return the recorded references for each distribution path.
		switch fpath {
		case "entrypoint.mjs":
			return []*block.BlockRef{profiledFirst, staleRef}, true, nil
		case "missing.mjs":
			return nil, false, nil
		case "chunk.mjs":
			return []*block.BlockRef{profiledSecond, profiledFirst}, true, nil
		default:
			t.Fatalf("unexpected path %q", fpath)
			return nil, false, nil
		}
	})

	// Replay the access profile over the available block candidates.
	got, err := ReplayAccessOrderRecord(ctx, graph, AccessOrderManifestIdentityFromRecord(record), record, []*block.BlockRef{
		fallbackChild,
		profiledSecond,
		fallbackRoot,
		profiledFirst,
	}, resolver)
	if err != nil {
		t.Fatalf("ReplayAccessOrderRecord: %v", err)
	}

	// Verify replay order and the reported stale or missing profile entries.
	assertRefOrder(t, got.Refs, []*block.BlockRef{profiledFirst, profiledSecond, fallbackRoot, fallbackChild})
	if !got.StaleRecord {
		t.Fatal("StaleRecord = false, want true for missing path and missing ref")
	}
	if got.UsedEntries != 2 {
		t.Fatalf("UsedEntries = %d, want 2", got.UsedEntries)
	}
	if !slices.Equal(got.MissingPaths, []string{"missing.mjs"}) {
		t.Fatalf("MissingPaths = %v, want [missing.mjs]", got.MissingPaths)
	}
	assertRefOrder(t, got.MissingRefs, []*block.BlockRef{staleRef})
}

func TestReplayAccessOrderRecordMissingRefsDoNotMakeRecordStale(t *testing.T) {
	// Create references for the access profile and fallback candidates.
	ctx := context.Background()
	present := testRef(t, "present-profiled")
	absent := testRef(t, "already-published")

	// Capture the profiled paths in their recorded access order.
	record := testAccessOrderRecord(t, []*AccessOrderEntry{
		{
			Filesystem:   AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST,
			Path:         "entrypoint.mjs",
			ResolvedRefs: []*block.BlockRef{present, absent},
		},
	})

	// Replay the access profile over the available block candidates.
	got, err := ReplayAccessOrderRecord(ctx, newTestRefGraph(), AccessOrderManifestIdentityFromRecord(record), record, []*block.BlockRef{present}, nil)
	if err != nil {
		t.Fatalf("ReplayAccessOrderRecord: %v", err)
	}

	// Verify replay order and the reported stale or missing profile entries.
	assertRefOrder(t, got.Refs, []*block.BlockRef{present})
	if got.StaleRecord {
		t.Fatal("StaleRecord = true, want false for refs outside the candidate set")
	}
	assertRefOrder(t, got.MissingRefs, []*block.BlockRef{absent})
}

func TestReplayAccessOrderRecordStaleMetadataUsesFallbackOrder(t *testing.T) {
	// Create references for the access profile and fallback candidates.
	ctx := context.Background()
	root := testRef(t, "stale-fallback-root")
	child := testRef(t, "stale-fallback-child")
	profiled := testRef(t, "stale-profiled")

	// Connect the fallback object root to its child block.
	graph := newTestRefGraph()
	graph.add(block_gc.ObjectIRI("fallback-object"), block_gc.BlockIRI(root))
	graph.add(block_gc.BlockIRI(root), block_gc.BlockIRI(child))

	// Capture the profiled paths in their recorded access order.
	record := testAccessOrderRecord(t, []*AccessOrderEntry{
		{
			Filesystem:   AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST,
			Path:         "entrypoint.mjs",
			ResolvedRefs: []*block.BlockRef{profiled},
		},
	})
	staleIdentity := AccessOrderManifestIdentityFromRecord(record)
	staleIdentity.ManifestRev++

	// Replay the access profile over the available block candidates.
	got, err := ReplayAccessOrderRecord(ctx, graph, staleIdentity, record, []*block.BlockRef{child, profiled, root}, nil)
	if err != nil {
		t.Fatalf("ReplayAccessOrderRecord: %v", err)
	}

	// Verify replay order and the reported stale or missing profile entries.
	assertRefOrder(t, got.Refs, []*block.BlockRef{root, child, profiled})
	if !got.StaleRecord {
		t.Fatal("StaleRecord = false, want true for stale manifest identity")
	}
	if got.UsedEntries != 0 {
		t.Fatalf("UsedEntries = %d, want 0", got.UsedEntries)
	}
	if len(got.MissingPaths) != 0 {
		t.Fatalf("MissingPaths = %v, want none", got.MissingPaths)
	}
	if len(got.MissingRefs) != 0 {
		t.Fatalf("MissingRefs = %v, want none", refKeys(got.MissingRefs))
	}
}

func testAccessOrderRecord(t *testing.T, entries []*AccessOrderEntry) *AccessOrderRecord {
	// Create a manifest record containing the supplied access entries.
	t.Helper()
	record := &AccessOrderRecord{
		ManifestId:      "manifest-1",
		PlatformId:      "darwin-arm64",
		BuildType:       "release",
		ManifestRootRef: testRef(t, "manifest-root"),
		ManifestRev:     7,
		Entries:         entries,
	}

	// Assign each access entry its position in the recorded sequence.
	for i, entry := range record.Entries {
		entry.Ordinal = uint64(i)
	}

	return record
}
