package provider_local

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// commitCountStore counts the write transactions a store commits.
type commitCountStore struct {
	kvtx.Store
	// commits counts committed write transactions.
	commits int
}

// NewTransaction wraps write transactions to count their commits.
func (s *commitCountStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &commitCountTx{Tx: tx, store: s}, nil
}

// commitCountTx records its commit on the store.
type commitCountTx struct {
	kvtx.Tx
	store *commitCountStore
}

// Commit counts the commit.
func (t *commitCountTx) Commit(ctx context.Context) error {
	t.store.commits++
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
	watch, lock, syncFuncs := NewObjectStoreSOStateFuncs(t.Context(), store, "", "")
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

// TestSOStateWritePublishedOperation checks that an ordinary local operation
// hands its state write to the publication in its context, and commits it
// itself when the publication declines.
func TestSOStateWritePublishedOperation(t *testing.T) {
	// Open a host over a commit-counting store that publishes as "objs".
	ctx := t.Context()
	store := &commitCountStore{Store: store_inmem.NewStore()}
	_, _, priv := seedTestGenesisState(t, store)
	watch, lock, syncFuncs := NewObjectStoreSOStateFuncs(ctx, store, "objs", "")
	host := sobject.NewSOHost(ctx, watch, lock, testSharedObjectID, syncFuncs)
	t.Cleanup(host.ClearContext)
	le := logrus.NewEntry(logrus.New())
	add := func(published bool) *block.AtomicHeadUpdate {
		// Add one operation through a publication that returns published.
		t.Helper()
		store.commits = 0
		var head *block.AtomicHeadUpdate
		publish := func(_ context.Context, h *block.AtomicHeadUpdate) (bool, error) {
			head = h
			return published, nil
		}
		pctx := sobject.WithPublishState(ctx, publish)
		if _, _, err := host.AddLocalOperation(pctx, le, testStepFactorySet(), priv, []byte("op")); err != nil {
			t.Fatal(err)
		}
		return head
	}

	// A declined publication commits the state itself.
	head := add(false)
	if head == nil || head.ObjectStoreID != "objs" || string(head.Key) != string(SobjectObjectStoreHostStateKey(testSharedObjectID)) {
		t.Fatalf("declined publication: head = %v", head)
	}
	if store.commits != 1 {
		t.Fatalf("declined publication: commits = %d", store.commits)
	}

	// An accepted publication leaves the state write to it.
	head = add(true)
	if store.commits != 0 {
		t.Fatalf("accepted publication: commits = %d", store.commits)
	}

	// The head replaces only the state the lock loaded.
	read, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	current, found, err := read.Get(ctx, head.Key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := head.Replace(ctx, current, found); err != nil {
		t.Fatalf("replace committed state: %v", err)
	}
	if _, err := head.Replace(ctx, nil, false); err == nil {
		t.Fatal("replace missing state succeeded")
	}
}
