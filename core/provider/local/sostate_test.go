package provider_local

import (
	"context"
	"testing"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	store_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
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

// TestSOStateWriteOrderedOperation checks that only an operation marked with
// sobject.WithOrderedOperation writes the host state with an ordered commit.
func TestSOStateWriteOrderedOperation(t *testing.T) {
	// Seed a signed root owned by the local peer.
	ctx := t.Context()
	priv, _, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	initial := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{{
			PeerId: pid.String(), Role: sobject.SOParticipantRole_SOParticipantRole_OWNER,
		}}},
		Root: &sobject.SORoot{InnerSeqno: 1, Inner: []byte("root")},
	}
	if err := initial.Root.SignInnerData(priv, testSharedObjectID, 1, hash.RecommendedHashType); err != nil {
		t.Fatal(err)
	}
	data, err := initial.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	store := &commitCountStore{Store: store_inmem.NewStore()}
	if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, SobjectObjectStoreHostStateKey(testSharedObjectID), data)
	}); err != nil {
		t.Fatal(err)
	}

	watch, lock, syncFuncs := NewObjectStoreSOStateFuncs(ctx, store, "")
	host := sobject.NewSOHost(ctx, watch, lock, testSharedObjectID, syncFuncs)
	t.Cleanup(host.ClearContext)
	queue := func(ctx context.Context) {
		t.Helper()
		store.full, store.ordered = 0, 0
		err := host.QueueOperation(ctx, pid, func(nonce uint64) (*sobject.SOOperation, error) {
			return sobject.BuildSOOperation(testSharedObjectID, priv, []byte("op"), nonce, ulid.NewULID())
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// A marked operation skips the full flush.
	queue(sobject.WithOrderedOperation(ctx))
	if store.full != 0 || store.ordered != 1 {
		t.Fatalf("marked operation: full = %d, ordered = %d", store.full, store.ordered)
	}

	// An unmarked operation keeps its full commit.
	queue(ctx)
	if store.full != 1 || store.ordered != 0 {
		t.Fatalf("unmarked operation: full = %d, ordered = %d", store.full, store.ordered)
	}
}
