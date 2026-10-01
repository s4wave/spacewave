package provider_spacewave

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/sirupsen/logrus"
)

// TestSyncCatalogCommitVisibility preserves accepted snapshots across failure and replay.
func TestSyncCatalogCommitVisibility(t *testing.T) {
	// Publish one pulled pack and pin a reader snapshot.
	ctx := t.Context()
	metadata := &syncMeasuredStore{store: newSyncTestKvStore()}
	catalog, err := manifest.New(ctx, metadata)
	if err != nil {
		t.Fatal(err)
	}
	syncer := &syncController{mfst: catalog, lower: packfile_store.NewPackfileStore(nil, nil)}
	initial := &packfile.PackfileEntry{Id: "original", Sequence: 8, BloomFilter: []byte("bloom"), BlockCount: 1}
	if err := syncer.applyManifestDelta(ctx, []*packfile.PackfileEntry{initial}, nil, 8); err != nil {
		t.Fatal(err)
	}
	pinned := syncer.lower.SnapshotManifest()

	// A failed commit publishes nothing to readers.
	injected := errors.New("failed manifest commit")
	metadata.commitErr = injected
	events := []*packfile.PackReplacementEvent{{Sequence: 10, ReplacedPackIds: []string{"original"}}}
	replacement := []*packfile.PackfileEntry{{Id: "replacement", Sequence: 9, BlockCount: 2}}
	if err := syncer.applyManifestDelta(ctx, replacement, events, 10); !errors.Is(err, injected) {
		t.Fatalf("commit error: %v", err)
	}
	if entries := syncer.lower.SnapshotManifest().GetEntries(); len(entries) != 1 || !entries[0].EqualVT(initial) {
		t.Fatal("failed durable change reached readers")
	}

	// The replay publishes the replacement without touching the pinned snapshot.
	metadata.commitErr = nil
	if err := syncer.applyManifestDelta(ctx, replacement, events, 10); err != nil {
		t.Fatal(err)
	}
	if entries := pinned.GetEntries(); len(entries) != 1 || !entries[0].EqualVT(initial) {
		t.Fatal("publication mutated an in-flight snapshot")
	}
	if entries := syncer.lower.SnapshotManifest().GetEntries(); len(entries) != 1 || !entries[0].EqualVT(replacement[0]) {
		t.Fatal("replacement was not published")
	}

	// Stale replay cannot replace server-authoritative metadata or regress its cursor.
	if err := syncer.applyManifestDelta(ctx, []*packfile.PackfileEntry{{Id: "replacement", Sequence: 7}}, nil, 7); err != nil {
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

// TestSyncPullRestartDropsUnlistedPacks verifies a restarted pull keeps its
// stored cursor until the final page, then drops the pulled packs the listing
// omitted and keeps locally authored ones.
func TestSyncPullRestartDropsUnlistedPacks(t *testing.T) {
	// Serve a restarted listing in two pages; the second page fails once.
	pages := map[string]*packfile.PullResponse{
		"5": {
			Entries:        []*packfile.PackfileEntry{{Id: "kept", Sequence: 4}, {Id: "fresh", Sequence: 20}},
			LatestSequence: 30,
			More:           true,
			Restart:        true,
		},
		"20": {
			Entries:        []*packfile.PackfileEntry{{Id: "newer", Sequence: 30}},
			LatestSequence: 30,
		},
	}
	failSecondPage := true

	// Serve the pages by their since cursor.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fail the first request for the second page.
		since := r.URL.Query().Get("since")
		if since == "20" && failSecondPage {
			failSecondPage = false
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// Write the requested page.
		data, err := pages[since].MarshalVT()
		if err != nil {
			t.Error(err)
		}
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	// Start from a stale cursor holding a pruned pack and a local pack.
	ctx := t.Context()
	catalog, err := manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatal(err)
	}
	priv, pid := generateTestKeypair(t)
	syncer := &syncController{
		client:     NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
		resourceID: "res-1",
		le:         logrus.NewEntry(logrus.New()),
		mfst:       catalog,
		lower:      packfile_store.NewPackfileStore(nil, nil),
	}

	// Store the held packs at cursor 5.
	held := []*packfile.PackfileEntry{{Id: "pruned", Sequence: 3}, {Id: "kept", Sequence: 4}, {Id: "local"}}
	if err := syncer.applyManifestDelta(ctx, held, nil, 5); err != nil {
		t.Fatal(err)
	}

	// An interrupted listing must leave the stale cursor in place.
	if err := syncer.pull(ctx); err == nil {
		t.Fatal("expected the failed second page to fail the pull")
	}
	if sequence, err := catalog.GetLastPullSequence(ctx); err != nil || sequence != 5 {
		t.Fatalf("interrupted cursor=%d err=%v, want 5", sequence, err)
	}

	// The completed listing drops the pruned pack and stores the head.
	if err := syncer.pull(ctx); err != nil {
		t.Fatal(err)
	}

	// List the remaining packs.
	var ids []string
	for _, entry := range catalog.GetEntries() {
		ids = append(ids, entry.GetId())
	}
	slices.Sort(ids)
	if want := []string{"fresh", "kept", "local", "newer"}; !slices.Equal(ids, want) {
		t.Fatalf("manifest = %v, want %v", ids, want)
	}
	if sequence, err := catalog.GetLastPullSequence(ctx); err != nil || sequence != 30 {
		t.Fatalf("cursor=%d err=%v, want 30", sequence, err)
	}
}
