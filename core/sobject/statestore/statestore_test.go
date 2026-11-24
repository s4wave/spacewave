package sobject_statestore

import (
	"context"
	"encoding/binary"
	"slices"
	"strings"
	"testing"

	"github.com/pkg/errors"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// testPrefix is the key prefix of the tested state.
var testPrefix = []byte("so/test/")

// testOp returns an operation whose body is i. The store does not verify
// signatures, so a distinct body is a distinct operation.
func testOp(i uint64) *sobject.SOOperation {
	inner := binary.BigEndian.AppendUint64([]byte("op"), i)
	return &sobject.SOOperation{Inner: inner}
}

// testPosition returns a position at height whose body is distinct per height
// and tag.
func testPosition(t testing.TB, height uint64, tag string) *sobject.SOSequence {
	inner, err := (&sobject.SOSequenceInner{Height: height, SharedObjectId: tag}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return &sobject.SOSequence{Inner: inner}
}

// testState returns a state holding n operations and n positions, each list
// in the order SOState keeps it.
func testState(t testing.TB, n int) *sobject.SOState {
	state := &sobject.SOState{
		Config:    &sobject.SharedObjectConfig{ConfigChainSeqno: 1},
		KeyEpochs: []*sobject.SOKeyEpoch{{Epoch: 0}},
	}
	for i := range n {
		state.Ops = append(state.Ops, testOp(uint64(i)))
		state.Sequence = append(state.Sequence, testPosition(t, uint64(i+1), "a"))
	}
	sortOps(state.Ops)
	return state
}

// sortOps sorts ops by hash, the order SOState keeps them in.
func sortOps(ops []*sobject.SOOperation) {
	slices.SortFunc(ops, func(a, b *sobject.SOOperation) int {
		return compareBytes(a.Hash(), b.Hash())
	})
}

// compareBytes compares two byte slices.
func compareBytes(a, b []byte) int {
	return slices.Compare(a, b)
}

// withOp returns a clone of state holding op in hash order.
func withOp(state *sobject.SOState, op *sobject.SOOperation) *sobject.SOState {
	next := state.Clone()
	next.Ops = append(next.Ops, op)
	sortOps(next.Ops)
	return next
}

// runTx runs fn in a write transaction on store.
func runTx(t *testing.T, store kvtx.Store, fn func(ctx context.Context, tx kvtx.Tx) error) {
	t.Helper()
	err := kvtx.RunTransaction(t.Context(), true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, fn)
	if err != nil {
		t.Fatal(err)
	}
}

// load reads the stored state and head from store.
func load(t *testing.T, store kvtx.Store) (*sobject.SOState, []byte) {
	t.Helper()
	var state *sobject.SOState
	var head []byte
	runTx(t, store, func(ctx context.Context, tx kvtx.Tx) error {
		var found bool
		var err error
		state, head, found, err = New(testPrefix).Load(ctx, tx)
		if err == nil && !found {
			t.Fatal("no state stored")
		}
		return err
	})
	return state, head
}

// TestStoreWritesReadBack writes a lineage of states through each kind of
// change and checks that every one loads back equal, and that a replace
// leaves none of the replaced state's keys.
func TestStoreWritesReadBack(t *testing.T) {
	// Store a state through Replace.
	s := New(testPrefix)
	store := store_kvtx_inmem.NewStore()
	state := testState(t, 4)
	var head []byte
	runTx(t, store, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		head, err = s.Replace(ctx, tx, state)
		return err
	})

	// write stores next over the held state and checks it reads back.
	write := func(next *sobject.SOState) {
		t.Helper()
		runTx(t, store, func(ctx context.Context, tx kvtx.Tx) error {
			var err error
			head, err = s.Write(ctx, tx, head, state, next)
			return err
		})
		loaded, loadedHead := load(t, store)
		if !loaded.EqualVT(next) {
			t.Fatalf("loaded state differs from the written state:\n%v\n%v", loaded, next)
		}
		if string(loadedHead) != string(head) {
			t.Fatal("loaded head differs from the written head")
		}
		state = next
	}

	// Add an operation, a position at a held height, and a control message.
	write(withOp(state, testOp(100)))
	next := state.Clone()
	next.Sequence = append(next.Sequence, testPosition(t, 1, "b"))
	slices.SortFunc(next.Sequence, func(a, b *sobject.SOSequence) int {
		return strings.Compare(mustSeqName(t, a), mustSeqName(t, b))
	})
	next.ControlMessages = []*sobject.SOControlMessage{{Inner: []byte("control")}}
	write(next)

	// Change the config, add a key epoch and set a checkpoint that drops
	// two operations and the first height's positions.
	next = state.Clone()
	next.Config.ConfigChainSeqno = 2
	next.KeyEpochs = append(next.KeyEpochs, &sobject.SOKeyEpoch{Epoch: 1})
	next.Checkpoint = &sobject.SOCheckpoint{Inner: []byte("checkpoint")}
	next.Ops = slices.Delete(next.Ops, 0, 2)
	next.Sequence = next.Sequence[2:]
	write(next)

	// Rewrite with deep copies: equal content stores nothing new but the head.
	before := countKeys(t, store)
	write(state.CloneVT())
	if after := countKeys(t, store); after != before {
		t.Fatalf("equal content changed the key count from %d to %d", before, after)
	}

	// Drop the checkpoint and every list.
	write(&sobject.SOState{Config: state.GetConfig()})

	// Replace with another state and check no stale key survives.
	other := testState(t, 2)
	runTx(t, store, func(ctx context.Context, tx kvtx.Tx) error {
		replaced, err := s.Replace(ctx, tx, other)
		if err == nil && string(replaced) == string(head) {
			t.Fatal("a replace kept the replaced state's head")
		}
		return err
	})
	if loaded, _ := load(t, store); !loaded.EqualVT(other) {
		t.Fatalf("replaced state differs:\n%v\n%v", loaded, other)
	}

	// Clear removes every key.
	runTx(t, store, func(ctx context.Context, tx kvtx.Tx) error {
		return s.Clear(ctx, tx)
	})
	if n := countKeys(t, store); n != 0 {
		t.Fatalf("clear left %d keys", n)
	}
}

// TestStoreWriteRefusesUnsortedLists checks that a write whose list is out of
// order stores nothing.
func TestStoreWriteRefusesUnsortedLists(t *testing.T) {
	// Write a state whose operations are in reverse hash order over an empty
	// state, so every entry is named and compared.
	next := testState(t, 3)
	slices.Reverse(next.Ops)
	tx := &countingTx{}
	if _, err := New(testPrefix).Write(t.Context(), tx, nil, &sobject.SOState{}, next); !errors.Is(err, ErrOutOfOrder) {
		t.Fatalf("want ErrOutOfOrder, got %v", err)
	}
}

// TestStoreWriteCostIsFlat holds a write's allocations and store calls flat
// between n and 10n entries above the checkpoint, for an appended operation,
// a placed position, and a checkpoint that covers one operation.
func TestStoreWriteCostIsFlat(t *testing.T) {
	// cost returns the allocations and store calls of writing next over prev.
	cost := func(prev, next *sobject.SOState) (float64, int) {
		tx := &countingTx{}
		s := New(testPrefix)
		allocs := testing.AllocsPerRun(20, func() {
			tx.calls = 0
			if _, err := s.Write(t.Context(), tx, nil, prev, next); err != nil {
				t.Fatal(err)
			}
		})
		return allocs, tx.calls
	}

	// Each case derives the next state from a state of n entries.
	cases := []struct {
		name string
		next func(state *sobject.SOState) *sobject.SOState
	}{
		{"append an operation", func(state *sobject.SOState) *sobject.SOState {
			return withOp(state, testOp(1<<40))
		}},
		{"place a position", func(state *sobject.SOState) *sobject.SOState {
			next := state.Clone()
			next.Sequence = append(next.Sequence, testPosition(t, uint64(len(state.Sequence)+1), "a"))
			return next
		}},
		{"checkpoint one operation", func(state *sobject.SOState) *sobject.SOState {
			next := state.Clone()
			next.Checkpoint = &sobject.SOCheckpoint{Inner: []byte("checkpoint")}
			next.Ops = slices.Delete(next.Ops, 0, 1)
			next.Sequence = next.Sequence[1:]
			return next
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			small, large := testState(t, 32), testState(t, 320)
			smallAllocs, smallCalls := cost(small, tc.next(small))
			largeAllocs, largeCalls := cost(large, tc.next(large))
			t.Logf("n=32: %v allocs, %d store calls; n=320: %v allocs, %d store calls", smallAllocs, smallCalls, largeAllocs, largeCalls)
			if largeAllocs > smallAllocs || largeCalls != smallCalls {
				t.Fatalf("cost grew from n to 10n: allocs %v to %v, store calls %d to %d",
					smallAllocs, largeAllocs, smallCalls, largeCalls)
			}
		})
	}
}

// mustSeqName returns the key name of a position.
func mustSeqName(t *testing.T, record *sobject.SOSequence) string {
	name, err := seqName(record)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

// countKeys returns the number of keys in store.
func countKeys(t *testing.T, store kvtx.Store) int {
	var n int
	runTx(t, store, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.ScanPrefixKeys(ctx, nil, func([]byte) error {
			n++
			return nil
		})
	})
	return n
}

// countingTx counts the store calls of a write and stores nothing.
type countingTx struct {
	kvtx.TxOps
	calls int
}

// Get counts a read and finds nothing.
func (c *countingTx) Get(context.Context, []byte) ([]byte, bool, error) {
	c.calls++
	return nil, false, nil
}

// Set counts a write.
func (c *countingTx) Set(context.Context, []byte, []byte) error {
	c.calls++
	return nil
}

// Delete counts a delete.
func (c *countingTx) Delete(context.Context, []byte) error {
	c.calls++
	return nil
}
