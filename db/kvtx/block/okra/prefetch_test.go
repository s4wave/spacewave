package kvtx_block_okra

import (
	"context"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// TestPrefetchWriteBatchLeavesTheBatchNoReads checks that a write batch reads
// no page after PrefetchWriteBatch of its keys, so a batch applied where a
// read must not wait, such as a commit under a writer lock, never reaches
// storage. Each batch updates, deletes and inserts keys spread over a stored
// tree, so it rebuilds windows that pull in preceding pages at several levels.
func TestPrefetchWriteBatchLeavesTheBatchNoReads(t *testing.T) {
	// Store a tree several levels deep on a store that records its reads.
	ctx := context.Background()
	store := &readOrderStore{StoreOps: newOkraTestStore()}
	fixture := newOkraFixture(t, ctx, store, 8192)
	built, _, err := BuildTree(ctx, store, nil, nil, fixture.seq())
	if err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := built.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// apply applies entries in a fresh write transaction on the stored tree,
	// prefetching their keys first when asked, and returns the batch's reads.
	apply := func(entries []kvtx.WriteBatchEntry, prefetch bool) []string {
		_, cursor := block.NewTransaction(store, nil, rootRef, nil)
		tx, err := NewTx(ctx, cursor, nil, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		if prefetch {
			keys := make([][]byte, len(entries))
			for i, e := range entries {
				keys[i] = e.Key
			}
			if err := tx.PrefetchWriteBatch(ctx, keys); err != nil {
				t.Fatal(err)
			}
		}
		store.takeReads()
		if err := tx.ApplyWriteBatch(ctx, entries); err != nil {
			t.Fatal(err)
		}
		return store.takeReads()
	}

	rng := rand.New(rand.NewPCG(3, 4))
	for round := range 8 {
		// Change about one key in twenty, some in long runs, so whole pages
		// and their first entries go.
		var entries []kvtx.WriteBatchEntry
		for i, key := range fixture.keys {
			pick := rng.IntN(64)
			if round%2 == 1 && (i/300)%7 == round%7 {
				pick = 1
			}
			switch pick {
			case 0:
				entries = append(entries, kvtx.WriteBatchEntry{Key: key, Value: []byte("update-" + strconv.Itoa(round))})
			case 1, 2:
				entries = append(entries, kvtx.WriteBatchEntry{Key: key, Delete: true})
			case 3:
				entries = append(entries, kvtx.WriteBatchEntry{Key: append(slices.Clone(key), '+'), Value: []byte("insert")})
			}
		}

		// The batch alone reads pages, and none after the prefetch.
		if reads := apply(entries, false); len(reads) == 0 {
			t.Fatalf("round %d: the batch read no page without a prefetch", round)
		}
		if reads := apply(entries, true); len(reads) != 0 {
			t.Fatalf("round %d: the batch read %d pages after the prefetch", round, len(reads))
		}
	}
}

// BenchmarkPrefetchWriteBatchWarm measures the prefetch of one held write
// whose pages are already decoded, the cost a write body pays under the
// writer lock for each held write in the common case.
func BenchmarkPrefetchWriteBatchWarm(b *testing.B) {
	// Store a tree and open a write transaction with every page decoded.
	ctx := context.Background()
	store := newOkraTestStore()
	fixture := newOkraFixture(b, ctx, store, 8192)
	built, _, err := BuildTree(ctx, store, nil, nil, fixture.seq())
	if err != nil {
		b.Fatal(err)
	}
	rootRef, _, err := built.Write(ctx, true)
	if err != nil {
		b.Fatal(err)
	}
	_, cursor := block.NewTransaction(store, nil, rootRef, nil)
	tx, err := NewTx(ctx, cursor, nil, true, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.PrefetchWriteBatch(ctx, fixture.keys); err != nil {
		b.Fatal(err)
	}

	// Prefetch one key per call, as a held write does.
	var i int
	for b.Loop() {
		if err := tx.PrefetchWriteBatch(ctx, fixture.keys[i%len(fixture.keys):][:1]); err != nil {
			b.Fatal(err)
		}
		i += 61
	}
}
