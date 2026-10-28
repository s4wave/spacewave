package block_test

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// TestDecodedBlockCacheSkipsStagedBlocks reproduces the decoded cache
// handing one transaction's staged block to another. A transaction reads back
// a block it staged but never published, then discards its staged writes. A
// second transaction sharing the lifecycle's decoded cache must not decode
// that block: no store holds it, and a commit that read it would link a ref
// to a missing block.
func TestDecodedBlockCacheSkipsStagedBlocks(t *testing.T) {
	// Share one decoded cache between transactions on one store, as an
	// object lifecycle does.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	// Stage a block in the first transaction and read it back through the
	// transaction's staged store.
	staging, cursor := block.NewTransaction(store, nil, nil, nil)
	staging.SetDecodedBlockCache(decodedBlocks)
	staged := staging.StageWrites(ctx, store)
	ref, _, err := block.PutBlock(ctx, staged, &block_mock.Example{Msg: "staged, never published"})
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor.SetRefAtCursor(ref, true)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatalf("staging transaction read its own block: %v", err)
	}

	// Discard the staged writes: the block never reaches the store.
	staging.DiscardStagedWrites()
	if found, err := store.GetBlockExists(ctx, ref); err != nil || found {
		t.Fatalf("store holds the discarded block: found=%v err=%v", found, err)
	}

	// A second transaction on the same store and cache must not find it.
	reader, readCursor := block.NewTransaction(store, nil, ref, nil)
	reader.SetDecodedBlockCache(decodedBlocks)
	blk, err := readCursor.Unmarshal(ctx, block_mock.NewExampleBlock)
	if !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("second transaction read a block no store holds: blk=%v err=%v", blk, err)
	}
}

// pendingOverlay serves blocks written through it from memory before a
// durable store, and never marks its reads. It stands in for any staging
// store nobody taught about the decoded-block cache.
type pendingOverlay struct {
	block.StoreOps
	pending map[string][]byte
}

// PutBlock keeps the block in memory only.
func (o *pendingOverlay) PutBlock(_ context.Context, data []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		return nil, false, err
	}
	o.pending[ref.MarshalString()] = data
	return ref, false, nil
}

// GetBlock serves a pending block before the durable store.
func (o *pendingOverlay) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if data, ok := o.pending[ref.MarshalString()]; ok {
		return data, true, nil
	}
	return o.StoreOps.GetBlock(ctx, ref)
}

// GetStoredBlock serves a pending block before the durable store.
func (o *pendingOverlay) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if data, ok := o.pending[ref.MarshalString()]; ok {
		return &block.StoredBlock{Data: data}, nil
	}
	return o.StoreOps.GetStoredBlock(ctx, ref)
}

// TestDecodedBlockCacheSkipsUnmarkedStagedStore tries to break the invariant
// with a staging store that never calls MarkStagedRead. Admission must not
// depend on staging stores reporting themselves: only a durable store's mark
// admits a block to the shared pool.
func TestDecodedBlockCacheSkipsUnmarkedStagedStore(t *testing.T) {
	// Share one decoded cache between transactions on one durable store.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	// Write a block to the overlay only and read it back through it.
	overlay := &pendingOverlay{StoreOps: store, pending: make(map[string][]byte)}
	ref, _, err := block.PutBlock(ctx, overlay, &block_mock.Example{Msg: "in memory, never stored"})
	if err != nil {
		t.Fatal(err.Error())
	}
	staging, cursor := block.NewTransaction(overlay, nil, ref, nil)
	staging.SetDecodedBlockCache(decodedBlocks)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatalf("overlay transaction read its own block: %v", err)
	}

	// A transaction on the durable store must not find it.
	reader, readCursor := block.NewTransaction(store, nil, ref, nil)
	reader.SetDecodedBlockCache(decodedBlocks)
	blk, err := readCursor.Unmarshal(ctx, block_mock.NewExampleBlock)
	if !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("durable store transaction read a block no store holds: blk=%v err=%v", blk, err)
	}
}

// TestDecodedBlockCacheDropsRemovedBlocks checks both halves of the
// invariant on a durable store: a block read from the store is shared with
// the next transaction, and once the store removes it, no transaction finds
// it.
func TestDecodedBlockCacheDropsRemovedBlocks(t *testing.T) {
	// Share one decoded cache between transactions on one durable store.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()
	read := func(ops block.StoreOps, ref *block.BlockRef) (block.Block, error) {
		tx, cursor := block.NewTransaction(ops, nil, ref, nil)
		tx.SetDecodedBlockCache(decodedBlocks)
		return cursor.Unmarshal(ctx, block_mock.NewExampleBlock)
	}

	// Store a block, read it, and read it again from the shared pool.
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "stored, then removed"})
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := read(store, ref); err != nil {
		t.Fatal(err.Error())
	}
	hits := decodedBlocks.Snapshot().Hits
	if _, err := read(store, ref); err != nil {
		t.Fatal(err.Error())
	}
	if got := decodedBlocks.Snapshot().Hits; got != hits+1 {
		t.Fatalf("second read of a stored block missed the pool: hits %d -> %d", hits, got)
	}

	// Remove the block from the store.
	if err := store.RmBlock(ctx, ref); err != nil {
		t.Fatal(err.Error())
	}

	// A transaction on the store must not find the removed block.
	blk, err := read(store, ref)
	if !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("transaction read a block the store removed: blk=%v err=%v", blk, err)
	}
}
