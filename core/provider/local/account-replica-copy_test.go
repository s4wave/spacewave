package provider_local

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
)

type replicaDestination struct {
	sobject.SharedObject
	store *BlockStore
}

func (r *replicaDestination) GetBlockStore() bstore.BlockStore {
	return r.store
}

// TestAccountReplicaCopyRetainsCompletedSubtrees copies a complete graph from a
// separate source. Durable completion proofs skip unchanged graphs on later heads.
func TestAccountReplicaCopyRetainsCompletedSubtrees(t *testing.T) {
	// Start a provider account and create a seeded Space.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, _, account, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	leaf, payload := seedAccountReplicaPayload(ctx, t, account, ref)

	// Mount it.
	so, releaseSO, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSO()
	states, releaseStates, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()

	// Decode its World head.
	inner, err := states.GetValue().GetCheckpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := &sobject_world_engine.InnerState{}
	if err := head.UnmarshalVT(inner.GetStateData()); err != nil {
		t.Fatal(err)
	}

	// Copying twice copies the graph once, in batches.
	local := newBatchForwardTestStore()
	destination := &replicaDestination{SharedObject: so, store: &BlockStore{
		store: local, readStore: so.GetBlockStore(),
	}}
	for i := range 2 {
		progress := &AccountReplicaCopyState{Head: head.GetHeadRef()}
		if err := account.copyAccountWorld(ctx, destination, progress, func() {}); err != nil {
			t.Fatal(err)
		}
		if i == 0 && progress.GetBlocks() == 0 {
			t.Fatal("copy did not visit the source graph")
		}
		if local.putBlockHits != 0 || (i == 0 && (local.putBlockBatchHits == 0 || uint64(local.putBlockBatchHits) >= progress.GetBlocks())) {
			t.Fatalf("copy of %d blocks used %d individual and %d batched writes", progress.GetBlocks(), local.putBlockHits, local.putBlockBatchHits)
		}
		if i == 1 && (progress.GetBlocks() != 0 || local.putBlockBatchHits != 0) {
			t.Fatal("completed immutable graph was copied again")
		}
		local.putBlockBatchHits = 0
	}

	// The local store holds the leaf.
	data, found, err := local.GetBlock(ctx, leaf)
	if err != nil || !found || string(data) != string(payload) {
		t.Fatalf("local leaf after copy: found=%v err=%v", found, err)
	}
}

// TestSharedObjectBlockStoreRetainsRoots checks that the write-back layer
// exposes the volume's root retention, so replacing a World head releases the
// superseded graph to the collector.
func TestSharedObjectBlockStoreRetainsRoots(t *testing.T) {
	// Start a provider account on a test volume.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, _, account, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Create and mount a Space shared object.
	ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	so, releaseSO, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSO()

	// Its block store must reach the volume's root retention.
	if !block.SupportsRootRetention(so.GetBlockStore()) {
		t.Fatal("shared object block store hides root retention")
	}
}

// TestWaitStateOrWake checks that a wake returns the unchanged state while a
// canceled caller still sees its own cancellation.
func TestWaitStateOrWake(t *testing.T) {
	// Hold an empty state that never changes.
	states := ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](nil)
	wake := func(context.Context) error { return nil }

	// A wake ends the wait without a new state.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := waitStateOrWake(ctx, states, nil, wake); err != nil {
		t.Fatalf("wake returned %v", err)
	}

	// A canceled caller is not a wake.
	canceled, cancelWait := context.WithCancel(ctx)
	cancelWait()
	hold := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	if _, err := waitStateOrWake(canceled, states, nil, hold); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait returned %v", err)
	}
}
