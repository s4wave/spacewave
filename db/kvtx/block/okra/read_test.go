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

// readOrderStore records the order of successful block reads, and the store
// calls that made them: one per single read and one per batch.
type readOrderStore struct {
	block.StoreOps
	// mtx guards reads and calls.
	mtx sync.Mutex
	// reads holds the hash of each block read.
	reads []string
	// calls counts the store calls.
	calls int
}

// GetBlock reads a block and records its hash.
func (s *readOrderStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := s.StoreOps.GetBlock(ctx, ref)
	s.record(ref, err == nil && found, true)
	return data, found, err
}

// GetStoredBlock reads a block and records its hash.
func (s *readOrderStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := s.StoreOps.GetStoredBlock(ctx, ref)
	s.record(ref, err == nil && stored != nil, true)
	return stored, err
}

// GetStoredBlockBatch reads refs as one store call and records each block.
func (s *readOrderStore) GetStoredBlockBatch(ctx context.Context, refs []*block.BlockRef, fn block.StoredBlockFunc) error {
	s.mtx.Lock()
	s.calls++
	s.mtx.Unlock()
	return block.GetStoredBlockEach(ctx, s.StoreOps, refs, func(i int, stored *block.StoredBlock, err error) error {
		s.record(refs[i], err == nil && stored != nil, false)
		return fn(i, stored, err)
	})
}

// record records a read of ref when found, counting a call when single.
func (s *readOrderStore) record(ref *block.BlockRef, found, single bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if single {
		s.calls++
	}
	if found {
		s.reads = append(s.reads, ref.GetHash().MarshalString())
	}
}

// takeCalls returns and clears the store call count.
func (s *readOrderStore) takeCalls() int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	calls := s.calls
	s.calls = 0
	return calls
}

// takeReads returns and clears the recorded reads.
func (s *readOrderStore) takeReads() []string {
	// Take and clear the recorded block reads under the store lock.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	reads := s.reads
	s.reads = nil
	return reads
}

func TestGetBatchReturnsValuesAndReadsTheSameBlocks(t *testing.T) {
	// Create a sorted fixture on a store that records block read order.
	ctx := context.Background()
	store := &readOrderStore{StoreOps: newOkraTestStore()}
	fixture := newOkraFixture(t, ctx, store, 4096)

	// Build and publish the fixture tree.
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

	// Compare batch values and block read order across fresh transactions.
	var firstReads []string
	for attempt := range 8 {
		// Open a fresh read transaction and resolve the shuffled batch.
		_, readCursor := block.NewTransaction(store, nil, rootRef, nil)
		okraTx, err := NewTx(ctx, readCursor, nil, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		store.takeReads()
		store.takeCalls()
		values, found, err := okraTx.GetBatch(ctx, keys)
		height := okraTx.root.GetHeight()
		okraTx.Discard()
		if err != nil {
			t.Fatal(err)
		}

		// Require the batch results to match all requested fixture values.
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

		// Require one store call per level: the root page alone, each level
		// of pages below it as one batch, and the leaves' values as one more.
		if calls := store.takeCalls(); calls > int(height)+2 {
			t.Fatalf("attempt %d made %d store calls for a tree of height %d", attempt, calls, height)
		}

		// Require each fresh transaction to read the same blocks. The batch
		// reads a level at once, so the order may differ.
		reads := store.takeReads()
		slices.Sort(reads)
		if attempt == 0 {
			firstReads = reads
			continue
		}
		if !slices.Equal(reads, firstReads) {
			t.Fatalf("attempt %d read %d blocks, not the blocks of the first attempt", attempt, len(reads))
		}
	}
}

func TestIteratorReadsEachParentsChildrenInOneBatch(t *testing.T) {
	// Publish a tree with more than one level of pages above its leaves.
	ctx := context.Background()
	store := &readOrderStore{StoreOps: newOkraTestStore()}
	fixture := newOkraFixture(t, ctx, store, 32768)
	tx, _, err := BuildTree(ctx, store, nil, nil, fixture.seq())
	if err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Scan the flat pages of a read and the cursor tree of a write, each way.
	for _, c := range []struct{ write, reverse bool }{{false, false}, {false, true}, {true, false}, {true, true}} {
		// Scan every key cold, counting the leaf pages the scan steps onto.
		write, reverse := c.write, c.reverse
		readTx := openOkraRoot(t, ctx, store, rootRef, write)
		height := readTx.root.GetHeight()
		store.takeReads()
		store.takeCalls()
		it := readTx.Iterate(ctx, nil, true, reverse).(*Iterator)
		var keys [][]byte
		var leaf walkPage
		leaves := 0
		for it.Next() {
			if page := it.walk.leaf(); page != leaf {
				leaf = page
				leaves++
			}
			keys = append(keys, bytes.Clone(it.Key()))
		}
		if err := it.Err(); err != nil {
			t.Fatal(err)
		}
		it.Close()
		readTx.Discard()

		// Require every key in the scan's order.
		want := slices.Clone(fixture.keys)
		if reverse {
			slices.Reverse(want)
		}
		if !slices.EqualFunc(keys, want, bytes.Equal) {
			t.Fatalf("write=%v reverse=%v scan returned %d keys, not the fixture's %d in order", write, reverse, len(keys), len(want))
		}

		// Require one store call per page of the seek's descent, then one per
		// page above the leaves: each parent's children in one batch.
		reads, calls := len(store.takeReads()), store.takeCalls()
		parents := reads - leaves
		t.Logf("write=%v reverse=%v height=%d leaves=%d parents=%d reads=%d calls=%d", write, reverse, height, leaves, parents, reads, calls)
		if height < 2 || leaves < 64 {
			t.Fatalf("fixture has height %d and %d leaves, too small to check", height, leaves)
		}
		if calls > int(height)+1+parents {
			t.Fatalf("write=%v reverse=%v scan of %d leaves under %d parents made %d store calls", write, reverse, leaves, parents, calls)
		}
	}
}
