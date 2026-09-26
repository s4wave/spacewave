package kvtx_block_okra

import (
	"bytes"
	"context"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// readOrderStore records the order of successful block reads.
type readOrderStore struct {
	block.StoreOps
	// mtx guards reads.
	mtx sync.Mutex
	// reads holds the hash of each block read.
	reads []string
}

// GetBlock reads a block and records its hash.
func (s *readOrderStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := s.StoreOps.GetBlock(ctx, ref)
	if err == nil && found {
		s.mtx.Lock()
		s.reads = append(s.reads, ref.GetHash().MarshalString())
		s.mtx.Unlock()
	}
	return data, found, err
}

// takeReads returns and clears the recorded reads.
func (s *readOrderStore) takeReads() []string {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	reads := s.reads
	s.reads = nil
	return reads
}

func TestGetBatchReturnsValuesAndReadsPagesInKeyOrder(t *testing.T) {
	ctx := context.Background()
	store := &readOrderStore{StoreOps: newOkraTestStore()}
	fixture := newOkraFixture(t, ctx, store, 4096)

	tx, _, err := BuildTree(ctx, store, nil, nil, fixture.seq())
	if err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Request every eighth key in shuffled order plus one absent key.
	var want [][]byte
	keys := [][]byte{[]byte("absent")}
	for i := 0; i < len(fixture.keys); i += 8 {
		keys = append(keys, fixture.keys[i])
		want = append(want, fixture.values[i])
	}
	rng := rand.New(rand.NewPCG(1, 2))
	rng.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })

	var firstReads []string
	for attempt := range 8 {
		_, readCursor := block.NewTransaction(store, nil, rootRef, nil)
		okraTx, err := NewTx(ctx, readCursor, nil, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		store.takeReads()
		values, found, err := okraTx.GetBatch(ctx, keys)
		okraTx.Discard()
		if err != nil {
			t.Fatal(err)
		}

		var got [][]byte
		for i, key := range keys {
			if bytes.Equal(key, []byte("absent")) {
				if found[i] {
					t.Fatal("absent key found")
				}
				continue
			}
			if !found[i] {
				t.Fatalf("key %q not found", key)
			}
			got = append(got, values[i])
		}
		slices.SortFunc(got, bytes.Compare)
		slices.SortFunc(want, bytes.Compare)
		if !slices.EqualFunc(got, want, bytes.Equal) {
			t.Fatal("batch values differ from the fixture")
		}

		reads := store.takeReads()
		if attempt == 0 {
			firstReads = reads
			continue
		}
		if !slices.Equal(reads, firstReads) {
			t.Fatalf("attempt %d read %d blocks in a different order than the first attempt", attempt, len(reads))
		}
	}
}
