package paylog

import (
	"context"
	"maps"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume/device"
	"github.com/s4wave/spacewave/net/hash"
)

// blockStore serves block operations in one scope: the store's own, whose
// puts and removes wait in the store's pending blocks, or a transaction's,
// whose puts and removes stay with the transaction until it ends.
type blockStore struct {
	// s is the store.
	s *Store
	// tx is the transaction, nil for the store's scope.
	tx *Tx
}

// GetHashType returns the hash type of new block references; zero selects
// the default.
func (b *blockStore) GetHashType() hash.HashType {
	return b.s.hashType
}

// GetSupportedFeatures reports native batches and the store's own buffering.
func (b *blockStore) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeatureNativeBatchPut | block.StoreFeatureNativeBatchExists | block.StoreFeatureSelfBuffered
}

// BeginReadOperation returns the scope, which needs no read scope.
func (b *blockStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return b, func() {}, nil
}

// PutBlock appends the payload to the open segment. The block is readable in
// the scope at once. The store's scope publishes it with the next Sync or
// index commit, a transaction's with its commit.
func (b *blockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Build and check the reference.
	if len(data) == 0 {
		return nil, false, block.ErrEmptyBlock
	}
	opts = opts.CloneVT()
	if opts == nil {
		opts = new(block.PutOpts)
	}
	opts.HashType = opts.SelectHashType(b.GetHashType())
	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	if forced := opts.GetForceBlockRef(); !forced.GetEmpty() && !ref.EqualsRef(forced) {
		return ref, false, block.ErrBlockRefMismatch
	}

	// Append the payload unless the block is already stored.
	existed, err := b.append(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data}})
	if err != nil {
		return ref, false, err
	}
	if opts.GetSync() {
		_, err = b.Sync(ctx)
	}
	return ref, existed[0], err
}

// PutBlockBatch verifies every entry, then appends the new payloads in one
// device call and records the tombstones as removes. Reports whether each block
// was stored before the write.
func (b *blockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	for _, entry := range entries {
		if entry == nil {
			return nil, block.ErrEmptyBlockRef
		}
		if err := entry.Ref.Validate(false); err != nil {
			return nil, err
		}
		if entry.Tombstone {
			continue
		}
		if len(entry.Data) == 0 {
			return nil, block.ErrEmptyBlock
		}
		if err := entry.Ref.VerifyData(entry.Data, false); err != nil {
			return nil, err
		}
	}
	return b.append(ctx, entries)
}

// append writes the payloads of entries not yet stored in one device call and
// adds their unpublished locations to the scope, and adds an unpublished
// remove for each tombstone. It returns whether each entry's block was stored
// before the call, false for a tombstone.
func (b *blockStore) append(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Only a write transaction or the store itself takes writes.
	s := b.s
	if b.tx != nil && !b.tx.write {
		return nil, kvtx.ErrNotWrite
	}

	// Resolve keys and find the published blocks in one index view.
	keys := make([]string, len(entries))
	stored := make([]bool, len(entries))
	for i, entry := range entries {
		key, err := blockKey(entry.Ref)
		if err != nil {
			return nil, err
		}
		keys[i] = key
	}
	err := b.view(ctx, func(tx kvtx.Tx) error {
		for i, entry := range entries {
			if entry.Tombstone {
				continue
			}
			var err error
			stored[i], err = tx.Exists(ctx, []byte(keys[i]))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Lay out the new payloads in a scope that is still open. mtx stays held
	// until they are issued and recorded.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if b.tx != nil && b.tx.own == nil {
		return nil, kvtx.ErrDiscarded
	}
	var writes []device.Write
	added := make(map[string]*pendingBlock)
	existed := make([]bool, len(entries))
	for i, entry := range entries {
		key := keys[i]
		if entry.Tombstone {
			added[key] = &pendingBlock{}
			continue
		}
		p := added[key]
		if p == nil {
			p = b.unpublished(key)
		}
		if (p != nil && p.loc != nil) || (p == nil && stored[i]) {
			existed[i] = true
			continue
		}
		loc := s.place(int64(len(entry.Data)))
		writes = append(writes, device.Write{Name: segmentName(loc.segment), Offset: loc.offset, Data: entry.Data})
		added[key] = &pendingBlock{loc: loc}
	}

	// Issue the payloads, then record them in the scope.
	if len(writes) != 0 {
		if err := s.dev.Write(ctx, writes, false); err != nil {
			return nil, err
		}
	}
	if b.tx != nil {
		maps.Copy(b.tx.own, added)
	} else {
		maps.Copy(s.pending, added)
	}
	return existed, nil
}

// unpublished returns the scope's unpublished change to the block at key: the
// transaction's own, then the store's pending one. The caller holds s.mtx.
func (b *blockStore) unpublished(key string) *pendingBlock {
	if b.tx != nil {
		if p := b.tx.own[key]; p != nil {
			return p
		}
	}
	return b.s.pending[key]
}

// view runs fn in the scope's index view: the transaction's, or a new read
// transaction for the store's scope.
func (b *blockStore) view(ctx context.Context, fn func(tx kvtx.Tx) error) error {
	if b.tx != nil {
		if b.tx.itx == nil {
			return kvtx.ErrDiscarded
		}
		return fn(b.tx.itx)
	}
	return b.s.view(ctx, fn)
}

// locate returns the locations of refs, nil for an absent block. It reads the
// unpublished changes before the index view, so an entry published in between
// is found in the view.
func (b *blockStore) locate(ctx context.Context, refs []*block.BlockRef) ([]*location, error) {
	// Resolve keys and answer from the unpublished changes.
	keys := make([]string, len(refs))
	found := make([]bool, len(refs))
	locs := make([]*location, len(refs))
	for i, ref := range refs {
		if ref.GetEmpty() {
			found[i] = true
			continue
		}
		key, err := blockKey(ref)
		if err != nil {
			return nil, err
		}
		keys[i] = key
	}
	b.s.mtx.Lock()
	for i, key := range keys {
		if p := b.unpublished(key); p != nil && !found[i] {
			locs[i], found[i] = p.loc, true
		}
	}
	b.s.mtx.Unlock()

	// Read the rest from one index view.
	err := b.view(ctx, func(tx kvtx.Tx) error {
		for i, key := range keys {
			if found[i] {
				continue
			}
			loc, err := getLocation(ctx, tx, []byte(key))
			if err != nil {
				return err
			}
			locs[i] = loc
		}
		return nil
	})
	return locs, err
}

// GetBlock reads a block's payload. A read that fails while maintenance
// moves the block retries at its new location.
func (b *blockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	refs := []*block.BlockRef{ref}
	locs, err := b.locate(ctx, refs)
	for err == nil && locs[0] != nil {
		// Read the payload from the segment.
		loc := locs[0]
		data := make([]byte, loc.length)
		read := []device.Read{{Name: segmentName(loc.segment), Offset: loc.offset, Data: data}}
		readErr := b.s.dev.Read(ctx, read)
		if readErr == nil {
			return data, true, nil
		}

		// Retry if the block moved or was removed meanwhile.
		locs, err = b.locate(ctx, refs)
		if err == nil && locs[0] != nil && *locs[0] == *loc {
			return nil, false, errors.Wrap(readErr, "read block")
		}
	}
	return nil, false, err
}

// GetStoredBlock serves the block without refs because this store keeps
// block bytes without their refs.
func (b *blockStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return block.GetBlockWithoutRefs(ctx, b, ref)
}

// GetBlockExists checks whether a block is stored.
func (b *blockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	locs, err := b.locate(ctx, []*block.BlockRef{ref})
	return err == nil && locs[0] != nil, err
}

// GetBlockExistsBatch checks each reference from one index view.
func (b *blockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Locate every reference and report which were found.
	locs, err := b.locate(ctx, refs)
	if err != nil {
		return nil, err
	}
	out := make([]bool, len(refs))
	for i, loc := range locs {
		out[i] = loc != nil
	}
	return out, nil
}

// StatBlock returns a block's payload length without reading it.
func (b *blockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	locs, err := b.locate(ctx, []*block.BlockRef{ref})
	if err != nil || locs[0] == nil {
		return nil, err
	}
	return &block.BlockStat{Ref: ref.Clone(), Size: locs[0].length}, nil
}

// RmBlock records an unpublished remove in the scope.
func (b *blockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	_, err := b.append(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}})
	return err
}

// Sync publishes the store's pending blocks. In a transaction's scope it
// reports durable, since the transaction's commit publishes its blocks.
func (b *blockStore) Sync(ctx context.Context) (bool, error) {
	if b.tx != nil {
		return true, nil
	}
	return true, b.s.sync(ctx)
}

// sync publishes every pending block write and remove in one index commit,
// which also makes the earlier ordered commits durable. With nothing pending
// it syncs the index instead, which costs nothing when nothing is dirty.
func (s *Store) sync(ctx context.Context) error {
	// Sync the index when nothing is pending.
	s.mtx.Lock()
	idle := len(s.pending) == 0
	s.mtx.Unlock()
	if idle {
		return s.index.Sync(ctx)
	}

	// Publish the pending writes and removes through one index commit.
	return s.update(ctx, false, nil)
}

// blockKey returns the index key of a block reference.
func blockKey(ref *block.BlockRef) (string, error) {
	if err := ref.Validate(false); err != nil {
		return "", err
	}
	key, err := ref.MarshalKey()
	if err != nil {
		return "", err
	}
	return blockPrefix + string(key), nil
}

// _ is a type assertion
var _ block.StoreOps = (*blockStore)(nil)
