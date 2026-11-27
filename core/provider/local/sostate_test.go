package provider_local

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// seedTestGenesisState returns the state, its genesis config change and the
// owner key.
func seedTestGenesisState(t *testing.T, store kvtx.Store) (*sobject.SOState, *sobject.SOConfigChange, crypto.PrivKey) {
	// Build the genesis state of a new owner.
	t.Helper()
	priv, _, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	le := logrus.NewEntry(logrus.New())
	state, genesis, err := sobject.BuildGenesisSOState(le, testStepFactorySet(), testSharedObjectID, priv, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Commit the state and its history.
	ctx := t.Context()
	if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		if err := WriteSOConfigHistory(ctx, tx, testSharedObjectID, genesis.GetConfig(), state.GetConfig(), []*sobject.SOConfigChange{genesis}); err != nil {
			return err
		}
		return ReplaceSOState(ctx, tx, testSharedObjectID, state)
	}); err != nil {
		t.Fatal(err)
	}
	return state, genesis, priv
}

// storeTestStateData stores the encoded state data as id's state in the
// selected layout, as another writer would.
func storeTestStateData(ctx context.Context, tx kvtx.Tx, id string, data []byte) error {
	state := &sobject.SOState{}
	if err := state.UnmarshalVT(data); err != nil {
		return err
	}
	return ReplaceSOState(ctx, tx, id, state)
}

// newTestGenesisHost returns a host over store seeded with the genesis state
// of a shared object owned by a fresh key, and that key.
func newTestGenesisHost(t *testing.T, store kvtx.Store) (*sobject.SOHost, crypto.PrivKey) {
	// Seed a genesis store and host it.
	t.Helper()
	_, _, priv := seedTestGenesisState(t, store)
	watch, lock, syncFuncs := NewObjectStoreSOStateFuncs(t.Context(), store, "", &SOStateRuns{Reload: func() {}}, stateLayout)
	host := sobject.NewSOHost(t.Context(), watch, lock, testSharedObjectID, syncFuncs)
	t.Cleanup(host.ClearContext)
	return host, priv
}

// testStepFactorySet returns the step factories of the default block transform.
func testStepFactorySet() *block_transform.StepFactorySet {
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	return sfs
}

// TestStateLayoutConversionKeepsEveryOp converts a state with signed ops from
// the whole layout to the log and back, writing through a real host in each
// layout, and tries to make the conversion lose, duplicate or invent an op:
// a stray part an earlier lifetime left, a conversion that never commits, and
// a state the log cannot hold.
func TestStateLayoutConversionKeepsEveryOp(t *testing.T) {
	// Seed a genesis in the whole layout.
	ctx := t.Context()
	selected := stateLayout
	t.Cleanup(func() { stateLayout = selected })
	stateLayout = SOStateLayoutWhole
	store := store_kvtx_inmem.NewStore()
	_, _, priv := seedTestGenesisState(t, store)
	le := logrus.NewEntry(logrus.New())

	// run runs fn in one transaction, committing it only if commit is set.
	run := func(commit bool, fn func(tx kvtx.Tx) error) error {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer tx.Discard()
		if err := fn(tx); err != nil || !commit {
			return err
		}
		return tx.Commit(ctx)
	}

	// write adds n signed ops through a host mounted in the selected layout
	// and returns the state it committed.
	write := func(n int) *sobject.SOState {
		t.Helper()
		hctx, cancel := context.WithCancel(ctx)
		defer cancel()
		watch, lock, syncFuncs := NewObjectStoreSOStateFuncs(hctx, store, "", &SOStateRuns{Reload: func() {}}, stateLayout)
		host := sobject.NewSOHost(hctx, watch, lock, testSharedObjectID, syncFuncs)
		defer host.ClearContext()
		for i := range n {
			if _, _, err := host.AddLocalOperation(ctx, le, testStepFactorySet(), priv, []byte{byte(i)}); err != nil {
				t.Fatal(err)
			}
		}
		state, err := host.GetHostState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return state
	}

	// stored loads the state from the selected layout and checks that the
	// other layout holds no key.
	stored := func() *sobject.SOState {
		t.Helper()
		var state *sobject.SOState
		err := run(false, func(tx kvtx.Tx) error {
			selected, other := stateLayout.layouts(testSharedObjectID)
			var found bool
			var err error
			if state, _, found, err = selected.load(ctx, tx); err == nil && !found {
				t.Fatal("no state in the selected layout")
			}
			if left, _, otherFound, _ := other.load(ctx, tx); otherFound {
				t.Fatalf("the other layout still holds a state of %d ops", len(left.GetOps()))
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return state
	}

	// convert runs the conversion to the selected layout.
	convert := func(commit bool) error {
		return run(commit, func(tx kvtx.Tx) error {
			_, err := convertSOState(ctx, tx, testSharedObjectID)
			return err
		})
	}

	// Write ops in the whole layout, then plant a valid op under the log's
	// prefix, as a crashed earlier lifetime could leave.
	whole := write(3)
	stray := &sobject.SOOperation{Inner: []byte("stray")}
	strayData, err := stray.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	strayKey := append(SobjectObjectStorePrefix(testSharedObjectID), "state/op/"+hex.EncodeToString(stray.Hash())...)
	if err := run(true, func(tx kvtx.Tx) error { return tx.Set(ctx, strayKey, strayData) }); err != nil {
		t.Fatal(err)
	}

	// A conversion that never commits leaves the whole layout as it was.
	stateLayout = SOStateLayoutLog
	if err := convert(false); err != nil {
		t.Fatal(err)
	}
	stateLayout = SOStateLayoutWhole
	if got := stored(); !got.EqualVT(whole) {
		t.Fatal("an uncommitted conversion changed the stored state")
	}

	// Convert to the log: the same ops, and no stray op.
	stateLayout = SOStateLayoutLog
	if err := convert(true); err != nil {
		t.Fatal(err)
	}
	if got := stored(); !got.EqualVT(whole) {
		t.Fatalf("the log holds another state than the whole layout did:\n%v\n%v", got, whole)
	}

	// A build older than the log layout reads only the host key. It must find
	// a key it fails to decode, never no state, which would start a genesis.
	host := SobjectObjectStoreHostStateKey(testSharedObjectID)
	if err := run(false, func(tx kvtx.Tx) error {
		data, found, err := tx.Get(ctx, host)
		if err != nil || !found {
			t.Fatalf("the log layout left no host key: %v", err)
		}
		if (&sobject.SOState{}).UnmarshalVT(data) == nil {
			t.Fatal("an older build decodes the log layout's host key")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Write through the log, then convert back: no write is lost.
	logged := write(2)
	if len(logged.GetOps()) != len(whole.GetOps())+2 {
		t.Fatalf("want %d ops after two log writes, got %d", len(whole.GetOps())+2, len(logged.GetOps()))
	}
	if got := stored(); !got.EqualVT(logged) {
		t.Fatal("the log stores another state than its host committed")
	}
	stateLayout = SOStateLayoutWhole
	if err := convert(true); err != nil {
		t.Fatal(err)
	}
	if got := stored(); !got.EqualVT(logged) {
		t.Fatal("converting back changed the state")
	}

	// A state the log cannot hold, here one with a field it does not know,
	// fails the conversion and keeps the whole layout.
	extended := protowire.AppendVarint(protowire.AppendTag(nil, 99, protowire.VarintType), 1)
	if err := run(true, func(tx kvtx.Tx) error {
		data, _, err := tx.Get(ctx, host)
		if err != nil {
			return err
		}
		return tx.Set(ctx, host, append(bytes.Clone(data), extended...))
	}); err != nil {
		t.Fatal(err)
	}
	stateLayout = SOStateLayoutLog
	if err := run(true, func(tx kvtx.Tx) error {
		_, err := convertSOState(ctx, tx, testSharedObjectID)
		return err
	}); !errors.Is(err, errStateLayoutMismatch) {
		t.Fatalf("want errStateLayoutMismatch, got %v", err)
	}
	stateLayout = SOStateLayoutWhole
	if got := stored(); len(got.GetOps()) != len(logged.GetOps()) {
		t.Fatal("a failed conversion changed the stored state")
	}
}
