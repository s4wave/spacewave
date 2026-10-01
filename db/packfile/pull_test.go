package packfile

import (
	"context"
	"slices"
	"testing"
)

// testPuller serves catalog pages keyed by their since cursor.
type testPuller map[uint64]*PullResponse

// SyncPull returns the page after since.
func (p testPuller) SyncPull(_ context.Context, _ string, since uint64) (*PullResponse, error) {
	return p[since], nil
}

// TestPullCatalogFollowsPagesAndRestarts verifies PullCatalog continues from
// each page's greatest sequence and discards the pages before a restart.
func TestPullCatalogFollowsPagesAndRestarts(t *testing.T) {
	// Serve two pages, a restart, and a final page.
	puller := testPuller{
		0: {Entries: []*PackfileEntry{{Id: "a", Sequence: 1}}, More: true, LatestSequence: 9},
		1: {Entries: []*PackfileEntry{{Id: "b", Sequence: 2}}, More: true, Restart: true, LatestSequence: 9},
		2: {
			Entries:           []*PackfileEntry{{Id: "c", Sequence: 9}},
			ReplacementEvents: []*PackReplacementEvent{{Sequence: 9, ReplacedPackIds: []string{"b"}}},
			LatestSequence:    9,
		},
	}

	// Read the whole catalog.
	catalog, err := PullCatalog(t.Context(), puller, "res-1")
	if err != nil {
		t.Fatal(err)
	}

	// The catalog holds the pages from the restart onward.
	var ids []string
	for _, entry := range catalog.GetEntries() {
		ids = append(ids, entry.GetId())
	}
	if !slices.Equal(ids, []string{"b", "c"}) || len(catalog.GetReplacementEvents()) != 1 || catalog.GetLatestSequence() != 9 {
		t.Fatalf("catalog = %v", catalog)
	}
}
