package provider_spacewave

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
)

// TestSyncCatalogCommitVisibility preserves accepted snapshots across failure and replay.
func TestSyncCatalogCommitVisibility(t *testing.T) {
	ctx := t.Context()
	metadata := &syncMeasuredStore{store: newSyncTestKvStore()}
	catalog, err := manifest.New(ctx, metadata)
	if err != nil {
		t.Fatal(err)
	}
	syncer := &syncController{mfst: catalog, lower: packfile_store.NewPackfileStore(nil, nil)}
	initial := &packfile.PackfileEntry{Id: "original", Sequence: 8, BloomFilter: []byte("bloom"), BlockCount: 1}
	if err := syncer.applyManifestDelta(ctx, []*packfile.PackfileEntry{initial}, nil); err != nil {
		t.Fatal(err)
	}
	pinned := syncer.lower.SnapshotManifest()
	injected := errors.New("failed manifest commit")
	metadata.commitErr = injected
	events := []*packfile.PackReplacementEvent{{Sequence: 10, ReplacedPackIds: []string{"original"}}}
	replacement := []*packfile.PackfileEntry{{Id: "replacement", Sequence: 9, BlockCount: 2}}
	if err := syncer.applyManifestDelta(ctx, replacement, events); !errors.Is(err, injected) {
		t.Fatalf("commit error: %v", err)
	}
	if entries := syncer.lower.SnapshotManifest().GetEntries(); len(entries) != 1 || !entries[0].EqualVT(initial) {
		t.Fatal("failed durable change reached readers")
	}
	metadata.commitErr = nil
	if err := syncer.applyManifestDelta(ctx, replacement, events); err != nil {
		t.Fatal(err)
	}
	if entries := pinned.GetEntries(); len(entries) != 1 || !entries[0].EqualVT(initial) {
		t.Fatal("publication mutated an in-flight snapshot")
	}
	if entries := syncer.lower.SnapshotManifest().GetEntries(); len(entries) != 1 || !entries[0].EqualVT(replacement[0]) {
		t.Fatal("replacement was not published")
	}

	// Stale replay cannot replace server-authoritative metadata or regress its cursor.
	if err := syncer.applyManifestDelta(ctx, []*packfile.PackfileEntry{{Id: "replacement", Sequence: 7}}, nil); err != nil {
		t.Fatal(err)
	}
	reopened, err := manifest.New(ctx, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if sequence, err := reopened.GetLastPullSequence(ctx); err != nil || sequence != 10 {
		t.Fatalf("reopened sequence=%d err=%v", sequence, err)
	}
	if entry := reopened.GetEntry("replacement"); !entry.EqualVT(replacement[0]) {
		t.Fatal("reopened catalog lost accepted metadata")
	}
}
