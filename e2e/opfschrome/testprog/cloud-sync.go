//go:build js

package main

import (
	"bytes"
	"context"
	"strconv"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	packfile_writer "github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

// probeDirtyTrackingStore records newly written blocks for the concurrent packing probe.
type probeDirtyTrackingStore struct {
	// store provides the underlying block operations.
	store block.StoreOps
	// dirtyStore stores the dirty index.
	dirtyStore kvtx.Store
	// flusher optionally packs blocks after the dirty-byte threshold.
	flusher *probeSyncFlusher
}

// GetHashType returns the underlying store's content hash algorithm.
func (d *probeDirtyTrackingStore) GetHashType() hash.HashType {
	return d.store.GetHashType()
}

// GetRoot reads a named root from the underlying store.
func (d *probeDirtyTrackingStore) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return d.store.GetRoot(ctx, name)
}

// ListRoots lists the underlying store's named roots.
func (d *probeDirtyTrackingStore) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return d.store.ListRoots(ctx, prefix, fn)
}

// SetRoots moves named roots on the underlying store.
func (d *probeDirtyTrackingStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return d.store.SetRoots(ctx, entries, roots)
}

// GetSupportedFeatures returns the underlying store's advertised capabilities.
func (d *probeDirtyTrackingStore) GetSupportedFeatures() block.StoreFeature {
	return d.store.GetSupportedFeatures()
}

// BeginReadOperation retains the underlying read scope while preserving dirty tracking.
func (d *probeDirtyTrackingStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := d.store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &probeDirtyTrackingStore{store: store, dirtyStore: d.dirtyStore, flusher: d.flusher}, release, nil
}

// PutBlock writes content and records a newly admitted block in the dirty store.
func (d *probeDirtyTrackingStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := d.store.PutBlock(ctx, data, opts)
	if err == nil && !existed {
		err = d.markDirty(ctx, ref.GetHash(), int64(len(data)))
	}
	return ref, existed, err
}

// PutBlockBatch writes the batch and records each previously absent live block.
func (d *probeDirtyTrackingStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Write the batch, then mark the previously absent live blocks dirty.
	existed, err := d.store.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	for i, entry := range entries {
		if entry == nil || entry.Tombstone || entry.Ref == nil || entry.Ref.GetEmpty() || existed[i] {
			continue
		}
		if err := d.markDirty(ctx, entry.Ref.GetHash(), int64(len(entry.Data))); err != nil {
			return nil, err
		}
	}
	return existed, nil
}

// GetBlock reads content from the underlying store.
func (d *probeDirtyTrackingStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return d.store.GetBlock(ctx, ref)
}

// GetStoredBlock forwards to the inner store.
func (d *probeDirtyTrackingStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return d.store.GetStoredBlock(ctx, ref)
}

// GetBlockExists delegates the underlying store's presence check.
func (d *probeDirtyTrackingStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return d.store.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch delegates a batch of presence checks.
func (d *probeDirtyTrackingStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return d.store.GetBlockExistsBatch(ctx, refs)
}

// RmBlock removes the block through the underlying store.
func (d *probeDirtyTrackingStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return d.store.RmBlock(ctx, ref)
}

// StatBlock returns the underlying store's block metadata.
func (d *probeDirtyTrackingStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return d.store.StatBlock(ctx, ref)
}

// Sync fences writes through the underlying store.
func (d *probeDirtyTrackingStore) Sync(ctx context.Context) (bool, error) {
	return d.store.Sync(ctx)
}

// markDirty commits the dirty entry before notifying the optional packer.
func (d *probeDirtyTrackingStore) markDirty(ctx context.Context, h *hash.Hash, size int64) error {
	// Record the block's size under its hash.
	tx, err := d.dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open dirty tx")
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte("dirty/"+h.MarshalString()), []byte(strconv.FormatInt(size, 10))); err != nil {
		return errors.Wrap(err, "set dirty key")
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit dirty key")
	}

	// Account the bytes with the packer, if any.
	if d.flusher != nil {
		d.flusher.markDirty(ctx, size)
	}
	return nil
}

const (
	// probeSyncSizeThresholdBytes starts packing after enough dirty data accumulates.
	probeSyncSizeThresholdBytes int64 = 48 * 1024 * 1024
	// probeSyncFlushMaxPackBytes bounds each in-memory pack payload.
	probeSyncFlushMaxPackBytes int64 = 4 * 1024 * 1024
)

// probeSyncFlusher starts one packing pass after dirty bytes cross its threshold.
type probeSyncFlusher struct {
	// upper reads dirty block payloads.
	upper block.StoreOps
	// dirtyStore provides the dirty-index transaction API.
	dirtyStore kvtx.Store
	// done reports the single packing pass's result.
	done chan error

	// mtx protects dirtySize and started.
	mtx sync.Mutex
	// dirtySize counts admitted dirty payload bytes under mtx.
	dirtySize int64
	// started prevents a second packing pass under mtx.
	started bool
}

// newProbeSyncFlusher constructs the single-pass dirty block packer.
func newProbeSyncFlusher(upper block.StoreOps, dirtyStore kvtx.Store) *probeSyncFlusher {
	return &probeSyncFlusher{
		upper:      upper,
		dirtyStore: dirtyStore,
		done:       make(chan error, 1),
	}
}

// markDirty accounts bytes and starts at most one background packing pass.
func (f *probeSyncFlusher) markDirty(ctx context.Context, size int64) {
	// Add the bytes and claim the pass once the threshold is crossed.
	f.mtx.Lock()
	f.dirtySize += size
	if f.started || f.dirtySize < probeSyncSizeThresholdBytes {
		f.mtx.Unlock()
		return
	}
	f.started = true
	f.mtx.Unlock()

	// Pack in the background.
	go func() {
		f.done <- f.flush(ctx)
	}()
}

// wait joins the packing pass if the threshold started one.
func (f *probeSyncFlusher) wait() error {
	// Join the pass only if one started.
	f.mtx.Lock()
	started := f.started
	f.mtx.Unlock()
	if !started {
		return nil
	}
	return <-f.done
}

// probeDirtyCandidate identifies a stored dirty block and its indexed byte length.
type probeDirtyCandidate struct {
	// hash identifies the stored block.
	hash *hash.Hash
	// size is its indexed payload length.
	size int64
}

// probeDirtyBlock holds one loaded dirty block until its pack is encoded.
type probeDirtyBlock struct {
	// hash identifies the loaded content.
	hash *hash.Hash
	// data holds the content until the current pack is encoded.
	data []byte
}

// flush packs dirty blocks in bounded chunks through the production pack writer.
func (f *probeSyncFlusher) flush(ctx context.Context) error {
	// Read the dirty index.
	candidates, err := f.scanDirty(ctx)
	if err != nil {
		return err
	}

	// Load and pack each bounded chunk in turn.
	maxBlocks := int(packfile_writer.DefaultPolicy().MaxBlocksPerPack)
	for start := 0; start < len(candidates); {
		end, err := nextProbeDirtyChunk(candidates, start, probeSyncFlushMaxPackBytes, maxBlocks)
		if err != nil {
			return err
		}
		blocks, err := f.loadDirtyBlocks(ctx, candidates[start:end])
		if err != nil {
			return err
		}
		if err := packProbeDirtyBlocks(blocks); err != nil {
			return err
		}
		start = end
	}
	return nil
}

// scanDirty reads the probe's dirty index from one metadata transaction.
func (f *probeSyncFlusher) scanDirty(ctx context.Context) ([]probeDirtyCandidate, error) {
	// Open a read transaction on the index.
	tx, err := f.dirtyStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "open dirty scan tx")
	}
	defer tx.Discard()

	// Decode each entry's hash and size. An unreadable size counts as zero.
	var out []probeDirtyCandidate
	prefix := []byte("dirty/")
	if err := tx.ScanPrefix(ctx, prefix, func(k, v []byte) error {
		// Parse the hash from the key.
		h := &hash.Hash{}
		if err := h.ParseFromB58(string(k[len(prefix):])); err != nil {
			return err
		}

		// Read the size.
		size, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil || size < 0 {
			size = 0
		}
		out = append(out, probeDirtyCandidate{hash: h, size: size})
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "scan dirty keys")
	}
	return out, nil
}

// loadDirtyBlocks loads the present blocks for one bounded candidate chunk.
func (f *probeSyncFlusher) loadDirtyBlocks(ctx context.Context, candidates []probeDirtyCandidate) ([]probeDirtyBlock, error) {
	// Skip candidates the upper store no longer holds.
	blocks := make([]probeDirtyBlock, 0, len(candidates))
	for _, candidate := range candidates {
		data, found, err := f.upper.GetBlock(ctx, block.NewBlockRef(candidate.hash))
		if err != nil {
			return nil, errors.Wrap(err, "get dirty block")
		}
		if !found {
			continue
		}
		blocks = append(blocks, probeDirtyBlock{hash: candidate.hash, data: data})
	}
	return blocks, nil
}

// nextProbeDirtyChunk selects a nonempty chunk within the pack writer's limits.
func nextProbeDirtyChunk(blocks []probeDirtyCandidate, start int, maxChunkBytes int64, maxChunkBlocks int) (int, error) {
	// Extend the chunk until a limit. An unknown size fills a whole chunk.
	var chunkBytes int64
	end := start
	for end < len(blocks) {
		size := blocks[end].size
		if size <= 0 {
			size = maxChunkBytes
		}
		if size > packfile_writer.DefaultMaxPackBytes {
			return 0, errors.Errorf("dirty block %s exceeds max pack chunk size", blocks[end].hash.MarshalString())
		}
		if maxChunkBlocks > 0 && end-start >= maxChunkBlocks {
			break
		}
		if chunkBytes > 0 && chunkBytes+size > maxChunkBytes {
			break
		}
		chunkBytes += size
		end++
	}

	// Always take at least one block.
	if end == start {
		end++
	}
	return end, nil
}

// packProbeDirtyBlocks encodes a chunk with the production pack format.
func packProbeDirtyBlocks(blocks []probeDirtyBlock) error {
	// Feed the blocks to the writer in order and discard the encoding.
	var buf bytes.Buffer
	idx := 0
	_, err := packfile_writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		dirty := blocks[idx]
		idx++
		return dirty.hash, &block.StoredBlock{Data: dirty.data, RefsSource: block.RefsAsserted}, nil
	})
	return errors.Wrap(err, "pack dirty blocks")
}
