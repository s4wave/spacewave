package kvtx_block_okra

import (
	"bytes"
	"context"
	"fmt"
	"math/bits"
	"math/rand/v2"
	"runtime"
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

// readCost is what one read of a tree cost: the store calls and the
// allocations it made.
type readCost struct {
	calls  int
	allocs uint64
}

// measureRead runs read and returns its store calls and allocations.
func measureRead(store *readOrderStore, read func()) readCost {
	store.takeReads()
	store.takeCalls()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	read()
	runtime.ReadMemStats(&after)
	return readCost{calls: store.takeCalls(), allocs: after.Mallocs - before.Mallocs}
}

func TestReadsScaleWithTheTree(t *testing.T) {
	// Read trees of n and 10n keys, one page per block and two levels per
	// block, recording the allocations per key and the store calls of each
	// read of the one-page tree.
	ctx := context.Background()
	perKey := make(map[string][]float64)
	for _, size := range []int{4096, 40960} {
		store := &readOrderStore{StoreOps: newOkraTestStore()}
		fixture := newOkraFixture(t, ctx, store, size)
		flatCalls, flatReads := make(map[string]int), make(map[string]int)
		for _, levels := range []uint32{0, 2} {
			// Publish a tree with more than one level of pages above its leaves.
			rootRef := buildScaleTree(t, ctx, store, fixture, levels)
			tag := fmt.Sprintf("levels=%d ", levels)

			// Look up every eighth key cold: the root alone, one batch per level
			// of pages below it, and one batch of the leaves' values.
			var keys [][]byte
			for i := 0; i < size; i += 8 {
				keys = append(keys, fixture.keys[i])
			}
			readTx := openOkraRoot(t, ctx, store, rootRef, false)
			height := readTx.root.GetHeight()
			cost := measureRead(store, func() {
				if _, _, err := readTx.GetBatch(ctx, keys); err != nil {
					t.Fatal(err)
				}
			})
			readTx.Discard()
			if cost.calls > int(height)+2 {
				t.Fatalf("%d keys: GetBatch of %d made %d store calls for height %d", size, len(keys), cost.calls, height)
			}
			perKey[tag+"lookup"] = append(perKey[tag+"lookup"], float64(cost.allocs)/float64(len(keys)))

			// Seek into the middle of the tree and read the value there, as a
			// prefix scan does: the descent follows children of pages it
			// loaded, below the root.
			mid := size / 2
			readTx = openOkraRoot(t, ctx, store, rootRef, false)
			var seen int
			if err := readTx.ScanPrefix(ctx, fixture.keys[mid], func(key, value []byte) error {
				if !bytes.Equal(key, fixture.keys[mid]) || !bytes.Equal(value, fixture.values[mid]) {
					t.Fatalf("%d keys: prefix scan of key %d returned another entry", size, mid)
				}
				seen++
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			readTx.Discard()
			if seen != 1 {
				t.Fatalf("%d keys: prefix scan of key %d returned %d entries", size, mid, seen)
			}

			// Step from a seek across one leaf boundary, as a range lookup does
			// to find its end: the step reads the next leaf, not the rest of
			// its parent.
			readTx = openOkraRoot(t, ctx, store, rootRef, false)
			it := readTx.Iterate(ctx, nil, true, false).(*Iterator)
			if err := it.Seek(fixture.keys[mid]); err != nil {
				t.Fatal(err)
			}
			seekLeaf := it.walk.leaf()
			store.takeReads()
			for it.Next() && it.walk.leaf() == seekLeaf {
			}
			if err := it.Err(); err != nil {
				t.Fatal(err)
			}
			crossed := len(store.takeReads())
			it.Close()
			readTx.Discard()
			if crossed > 2 {
				t.Fatalf("%d keys: %sstep past the seek's leaf read %d blocks", size, tag, crossed)
			}

			// Scan the flat pages of a read and the cursor tree of a write, each
			// way: one store call per page of the seek's descent, then one per
			// page above the leaves once each level's read-ahead has grown to
			// a whole page, which takes about log2 of the fan-out batches per
			// level.
			for _, c := range []struct{ write, reverse bool }{{false, false}, {false, true}, {true, false}, {true, true}} {
				name := fmt.Sprintf("scan write=%v reverse=%v", c.write, c.reverse)
				readTx := openOkraRoot(t, ctx, store, rootRef, c.write)
				var got [][]byte
				var leaf walkPage
				leaves := 0
				cost := measureRead(store, func() {
					it := readTx.Iterate(ctx, nil, true, c.reverse).(*Iterator)
					defer it.Close()
					for it.Next() {
						if page := it.walk.leaf(); page != leaf {
							leaf = page
							leaves++
						}
						got = append(got, bytes.Clone(it.Key()))
					}
					if err := it.Err(); err != nil {
						t.Fatal(err)
					}
				})
				readTx.Discard()

				// Require every key in the scan's order.
				want := slices.Clone(fixture.keys)
				if c.reverse {
					slices.Reverse(want)
				}
				if !slices.EqualFunc(got, want, bytes.Equal) {
					t.Fatalf("%d keys: %s returned %d keys, not the fixture's in order", size, name, len(got))
				}
				reads := len(store.takeReads())
				if height < 2 || leaves < 64 {
					t.Fatalf("%d keys: height %d and %d leaves are too small to check", size, height, leaves)
				}
				if levels == 0 {
					flatCalls[name], flatReads[name] = cost.calls, reads
					growth := int(height) * bits.Len(uint(leaves))
					if parents := reads - leaves; cost.calls > int(height)+1+parents+growth {
						t.Fatalf("%d keys: %s of %d leaves under %d parents made %d store calls", size, name, leaves, parents, cost.calls)
					}
				} else if cost.calls > flatCalls[name] {
					// Embedded pages arrive with their parent's block, so the
					// layout never needs more store calls than one page per block.
					t.Fatalf("%d keys: %s at %d levels per block made %d store calls, %d at one", size, name, levels, cost.calls, flatCalls[name])
				} else if reads >= flatReads[name] {
					t.Fatalf("%d keys: %s at %d levels per block read %d blocks, %d at one", size, name, levels, reads, flatReads[name])
				}
				t.Logf("%d keys: %s%s: %d leaves, %d blocks, %d store calls, %.1f allocations per key", size, tag, name, leaves, reads, cost.calls, float64(cost.allocs)/float64(size))
				perKey[tag+name] = append(perKey[tag+name], float64(cost.allocs)/float64(size))
			}
		}
	}

	// Require the allocations per key to stay flat from n to 10n keys.
	for name, costs := range perKey {
		if costs[1] > 1.5*costs[0] {
			t.Fatalf("%s: %.1f allocations per key at 10n, %.1f at n", name, costs[1], costs[0])
		}
	}
}

// buildScaleTree publishes the fixture's keys and value refs as a tree that
// stores levels levels of pages per block, and returns its root.
func buildScaleTree(t *testing.T, ctx context.Context, store block.StoreOps, fixture *okraFixture, levels uint32) *block.BlockRef {
	t.Helper()
	if levels == 0 {
		tx, _, err := BuildTree(ctx, store, nil, nil, fixture.seq())
		if err != nil {
			t.Fatal(err)
		}
		rootRef, _, err := tx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		return rootRef
	}
	return writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		tx.levelsPerBlock = levels
		if err := tx.ReplaceAll(ctx, func(yield func([]byte, []byte) bool) {
			for i, key := range fixture.keys {
				if !yield(key, fixture.values[i]) {
					return
				}
			}
		}); err != nil {
			t.Fatal(err)
		}
	})
}
