package provider_local

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/blocktype"
	blocktype_controller "github.com/s4wave/spacewave/db/blocktype/controller"
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
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, _, account, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	decoder := blocktype_controller.NewController(func(_ context.Context, typeID string) (blocktype.BlockType, error) {
		if typeID == "test/replica-payload" {
			return blocktype.NewBlockType(typeID, block_mock.NewRootBlock), nil
		}
		return nil, nil
	})
	releaseDecoder, err := account.t.p.b.AddController(ctx, decoder, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseDecoder()
	ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	leaf, payload := seedAccountReplicaPayload(ctx, t, account, ref)
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
	inner, err := states.GetValue().GetRootInner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := &sobject_world_engine.InnerState{}
	if err := head.UnmarshalVT(inner.GetStateData()); err != nil {
		t.Fatal(err)
	}
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
	data, found, err := local.GetBlock(ctx, leaf)
	if err != nil || !found || string(data) != string(payload) {
		t.Fatalf("local leaf after copy: found=%v err=%v", found, err)
	}
}
