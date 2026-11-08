package block_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// refsUnknownStore serves every block as a peer does that holds it without
// its refs, asking the reader to hand them over, and counts the hand-overs.
type refsUnknownStore struct {
	block.StoreOps
	kept int
}

func (s *refsUnknownStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := s.StoreOps.GetBlock(ctx, ref)
	if found {
		block.KeepDecodedRefs(ctx, ref, data, func(context.Context, []byte, []*block.BlockRef) { s.kept++ })
	}
	return data, found, err
}

// TestFetchKeepsRefsUnknownBlocks checks which reads keep a block a store
// served without its refs: Unmarshal and FetchLeaf keep it, FetchWithKeep
// hands the keep to its caller, and Fetch drops it. A read that drops it asks
// the peer again on every later read.
func TestFetchKeepsRefsUnknownBlocks(t *testing.T) {
	ctx := context.Background()
	store := &refsUnknownStore{StoreOps: block_mock.NewMockStore(0)}
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "peer"})
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor := func() *block.Cursor {
		_, c := block.NewTransaction(store, nil, ref, nil)
		return c
	}

	if _, _, err := cursor().Fetch(ctx); err != nil || store.kept != 0 {
		t.Fatalf("Fetch: err %v, kept %d", err, store.kept)
	}
	if _, err := cursor().Unmarshal(ctx, func() block.Block { return &block_mock.Example{} }); err != nil || store.kept != 1 {
		t.Fatalf("Unmarshal: err %v, kept %d", err, store.kept)
	}
	if _, _, err := cursor().FetchLeaf(ctx); err != nil || store.kept != 2 {
		t.Fatalf("FetchLeaf: err %v, kept %d", err, store.kept)
	}
	_, keep, found, err := cursor().FetchWithKeep(ctx)
	if err != nil || !found || keep == nil {
		t.Fatalf("FetchWithKeep: err %v, found %v, keep %v", err, found, keep != nil)
	}
	keep(ctx, nil)
	if store.kept != 3 {
		t.Fatalf("FetchWithKeep keep: kept %d", store.kept)
	}
}
