package provider_spacewave

import (
	"slices"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
)

// TestSyncCatalogDeltaTransitions publishes only committed accepted metadata.
func TestSyncCatalogDeltaTransitions(t *testing.T) {
	// Open the durable manifest with injectable commit failures.
	ctx := t.Context()
	backend := &syncMeasuredStore{store: newSyncTestKvStore()}
	catalog, err := manifest.New(ctx, backend)
	if err != nil {
		t.Fatal(err)
	}

	// Start a provider with one remote snapshot acquisition at initialization.
	remoteReads := 0
	syncer := &syncController{
		mfst:  catalog,
		lower: packfile_store.NewPackfileStore(nil, nil),
		remote: func() []*packfile.PackfileEntry {
			remoteReads++
			return nil
		},
	}
	t.Cleanup(syncer.lower.Close)
	syncer.publishManifest()

	// Prepare accepted descriptors and replacement events for each transition.
	original := &packfile.PackfileEntry{Id: "original", Sequence: 7, BlockCount: 1}
	kept := &packfile.PackfileEntry{Id: "kept", Sequence: 8, BlockCount: 2}
	replacement := &packfile.PackfileEntry{Id: "replacement", Sequence: 10, BlockCount: 3}
	retire := []*packfile.PackReplacementEvent{{Sequence: 11, ReplacedPackIds: []string{"original"}}}
	var expected []*packfile.PackfileEntry
	var cursor uint64
	notifications := 0

	// Compare each observer notification with durable state and its pull cursor.
	check := func() {
		// Read the published catalog in the durable manifest's ID order.
		t.Helper()
		read := syncer.lower.SnapshotManifest().GetEntries()
		slices.SortFunc(read, func(a, b *packfile.PackfileEntry) int {
			return strings.Compare(a.GetId(), b.GetId())
		})
		if !slices.EqualFunc(read, expected, func(a, b *packfile.PackfileEntry) bool { return a.EqualVT(b) }) {
			t.Fatalf("published entries = %v, want %v", read, expected)
		}

		// Compare the observer's state with a freshly opened manifest.
		reopened, err := manifest.New(ctx, backend)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.EqualFunc(reopened.GetEntries(), expected, func(a, b *packfile.PackfileEntry) bool { return a.EqualVT(b) }) {
			t.Fatal("publication differs from reopened durable catalog")
		}
		if sequence, err := reopened.GetLastPullSequence(ctx); err != nil || sequence != cursor {
			t.Fatalf("durable cursor=%d err=%v, want %d", sequence, err, cursor)
		}
	}

	// Check durable state from inside each publication notification.
	syncer.lower.SetStatsChangedCallback(func() {
		notifications++
		check()
	})

	// Exercise addition, replay, local precedence, replacement, and deletion.
	for _, step := range []struct {
		// name identifies the transition.
		name string
		// entries carries changed descriptors.
		entries []*packfile.PackfileEntry
		// events carries replacement removals.
		events []*packfile.PackReplacementEvent
		// cursor advances the durable pull cursor.
		cursor uint64
		// expected is the complete accepted catalog in ID order.
		expected []*packfile.PackfileEntry
	}{
		{"add", []*packfile.PackfileEntry{original, kept}, nil, 8, []*packfile.PackfileEntry{kept, original}},
		{"replay", []*packfile.PackfileEntry{original, kept}, nil, 8, []*packfile.PackfileEntry{kept, original}},
		{"local", []*packfile.PackfileEntry{{Id: "original", BlockCount: 99}}, nil, 0, []*packfile.PackfileEntry{kept, original}},
		{"local-delete", []*packfile.PackfileEntry{{Id: "original", SupersededBy: "obsolete"}}, nil, 0, []*packfile.PackfileEntry{kept, original}},
		{"stale-delete", []*packfile.PackfileEntry{{Id: "original", Sequence: 6, SupersededBy: "obsolete"}}, nil, 6, []*packfile.PackfileEntry{kept, original}},
		{"replace", []*packfile.PackfileEntry{replacement}, retire, 11, []*packfile.PackfileEntry{kept, replacement}},
		{"delete", []*packfile.PackfileEntry{{Id: "replacement", Sequence: 12, SupersededBy: "retired"}}, nil, 12, []*packfile.PackfileEntry{kept}},
	} {
		expected = step.expected
		cursor = max(cursor, step.cursor)
		if err := syncer.applyManifestDelta(ctx, step.entries, step.events, step.cursor); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		check()
	}

	// Seed the unaffected pack's durable index before the failing deletion.
	cache := manifest.NewIndexCache(backend)
	if err := cache.Set(ctx, "kept", []byte("kept-index")); err != nil {
		t.Fatal(err)
	}

	// Failed durability leaves the read store, observer, cursor, and reopened catalog intact.
	before := notifications
	injected := errors.New("catalog commit failed")
	backend.commitErr = injected
	if err := syncer.applyManifestDelta(ctx, nil, []*packfile.PackReplacementEvent{{Sequence: 13, ReplacedPackIds: []string{"kept"}}}, 13); !errors.Is(err, injected) {
		t.Fatalf("failed commit returned %v", err)
	}
	check()
	if tail, found, err := cache.Get(ctx, "kept"); err != nil || !found || string(tail) != "kept-index" {
		t.Fatalf("failed commit changed index: tail=%q found=%v err=%v", tail, found, err)
	}
	if notifications != before || remoteReads != 1 {
		t.Fatalf("failed commit notified or delta rebuilt remote snapshot: notifications=%d/%d remote-reads=%d", notifications, before, remoteReads)
	}
}
