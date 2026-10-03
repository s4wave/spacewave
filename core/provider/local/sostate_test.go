package provider_local

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// commitCountStore counts how the write transactions of a store commit.
type commitCountStore struct {
	kvtx.Store
	// full counts write transactions finished by Commit.
	full int
	// ordered counts write transactions finished by CommitOrdered.
	ordered int
}

// NewTransaction wraps write transactions to count their commits.
func (s *commitCountStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &commitCountTx{Tx: tx, store: s}, nil
}

// commitCountTx records its commit kind on the store.
type commitCountTx struct {
	kvtx.Tx
	store *commitCountStore
}

// Commit counts a full commit.
func (t *commitCountTx) Commit(ctx context.Context) error {
	t.store.full++
	return t.Tx.Commit(ctx)
}

// CommitOrdered counts an ordered commit.
func (t *commitCountTx) CommitOrdered(ctx context.Context) error {
	t.store.ordered++
	return t.Tx.Commit(ctx)
}

// seedTestGenesisState writes to store the genesis state of a shared object
// owned by a fresh key, with its config history, as a new object is created.
// It returns the state, its genesis config change and the owner key.
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

	// Commit the encoded state and its history.
	data, err := state.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		if err := WriteSOConfigHistory(ctx, tx, testSharedObjectID, genesis.GetConfig(), state.GetConfig(), []*sobject.SOConfigChange{genesis}); err != nil {
			return err
		}
		return tx.Set(ctx, SobjectObjectStoreHostStateKey(testSharedObjectID), data)
	}); err != nil {
		t.Fatal(err)
	}
	return state, genesis, priv
}

// newTestGenesisHost returns a host over store seeded with the genesis state
// of a shared object owned by a fresh key, and that key.
func newTestGenesisHost(t *testing.T, store kvtx.Store) (*sobject.SOHost, crypto.PrivKey) {
	// Seed a genesis store and host it.
	t.Helper()
	_, _, priv := seedTestGenesisState(t, store)
	watch, lock, syncFuncs := NewObjectStoreSOStateFuncs(t.Context(), store, "")
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

// TestSOStateWriteOrderedOperation checks that only an operation marked with
// sobject.WithOrderedOperation writes the host state with an ordered commit.
func TestSOStateWriteOrderedOperation(t *testing.T) {
	// Open a host over a commit-counting store.
	ctx := t.Context()
	store := &commitCountStore{Store: store_inmem.NewStore()}
	host, priv := newTestGenesisHost(t, store)
	le := logrus.NewEntry(logrus.New())
	add := func(ctx context.Context) {
		// Reset the counters and add one operation.
		t.Helper()
		store.full, store.ordered = 0, 0
		if _, _, err := host.AddLocalOperation(ctx, le, testStepFactorySet(), priv, []byte("op")); err != nil {
			t.Fatal(err)
		}
	}

	// A marked operation skips the full flush.
	add(sobject.WithOrderedOperation(ctx))
	if store.full != 0 || store.ordered != 1 {
		t.Fatalf("marked operation: full = %d, ordered = %d", store.full, store.ordered)
	}

	// An unmarked operation keeps its full commit.
	add(ctx)
	if store.full != 1 || store.ordered != 0 {
		t.Fatalf("unmarked operation: full = %d, ordered = %d", store.full, store.ordered)
	}
}
