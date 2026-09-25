package direct

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume/memtable"
	"github.com/s4wave/spacewave/net/hash"
)

// GetHashType returns zero to select the default hash type.
func (s *Store) GetHashType() hash.HashType {
	return 0
}

// GetSupportedFeatures reports native batches and the store's own buffering.
func (s *Store) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeatureNativeBatchPut | block.StoreFeatureNativeBatchExists | block.StoreFeatureSelfBuffered
}

// BeginReadOperation returns the store, which needs no read scope.
func (s *Store) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// PutBlock adds the payload to the pending blocks. The block is readable at
// once and durable after the next Sync or durable commit.
func (s *Store) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Build and check the reference.
	if len(data) == 0 {
		return nil, false, block.ErrEmptyBlock
	}
	opts = opts.CloneVT()
	if opts == nil {
		opts = new(block.PutOpts)
	}
	opts.HashType = opts.SelectHashType(s.GetHashType())
	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	if forced := opts.GetForceBlockRef(); !forced.GetEmpty() && !ref.EqualsRef(forced) {
		return ref, false, block.ErrBlockRefMismatch
	}

	// Add the payload unless the block is already stored.
	added, err := s.add(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data}})
	if err != nil {
		return ref, false, err
	}
	if opts.GetSync() {
		_, err = s.Sync(ctx)
	}
	return ref, added == 0, err
}

// PutBlockBatch verifies every entry, then adds the new payloads and the
// tombstones as removes.
func (s *Store) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	for _, entry := range entries {
		if entry == nil {
			return block.ErrEmptyBlockRef
		}
		if err := entry.Ref.Validate(false); err != nil {
			return err
		}
		if entry.Tombstone {
			continue
		}
		if len(entry.Data) == 0 {
			return block.ErrEmptyBlock
		}
		if err := entry.Ref.VerifyData(entry.Data, false); err != nil {
			return err
		}
	}
	_, err := s.add(ctx, entries)
	return err
}

// add adds a pending write for each entry not yet stored and a pending remove
// for each tombstone. It returns the number of writes added.
func (s *Store) add(ctx context.Context, entries []*block.PutBatchEntry) (int, error) {
	// Resolve keys and find the stored blocks in one call.
	keys := make([][]byte, len(entries))
	var puts [][]byte
	for i, entry := range entries {
		key, err := blockKey(entry.Ref)
		if err != nil {
			return 0, err
		}
		keys[i] = key
		if !entry.Tombstone {
			puts = append(puts, key)
		}
	}
	var stored []bool
	if len(puts) != 0 {
		var err error
		stored, err = s.records.Has(ctx, puts)
		if err != nil {
			return 0, err
		}
	}

	// Record the writes and removes.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	var added int
	for i, entry := range entries {
		key := string(keys[i])
		if entry.Tombstone {
			s.pending[key] = &pendingBlock{}
			continue
		}
		wasStored := stored[0]
		stored = stored[1:]
		p := s.pending[key]
		if (p != nil && p.data != nil) || (p == nil && wasStored) {
			continue
		}
		s.pending[key] = &pendingBlock{data: entry.Data}
		added++
	}
	return added, nil
}

// lookup returns the pending entries of refs and, for the rest, whether they
// are stored, reading the payloads if read is set. It reads pending entries
// before the record store, so an entry committed in between is found there.
func (s *Store) lookup(ctx context.Context, refs []*block.BlockRef, read bool) ([][]byte, []bool, error) {
	// Resolve keys and answer from pending entries.
	keys := make([][]byte, len(refs))
	data := make([][]byte, len(refs))
	found := make([]bool, len(refs))
	done := make([]bool, len(refs))
	for i, ref := range refs {
		if ref.GetEmpty() {
			done[i] = true
			continue
		}
		key, err := blockKey(ref)
		if err != nil {
			return nil, nil, err
		}
		keys[i] = key
	}
	s.mtx.Lock()
	for i, key := range keys {
		if p := s.pending[string(key)]; p != nil && !done[i] {
			data[i], found[i], done[i] = p.data, p.data != nil, true
		}
	}
	s.mtx.Unlock()

	// Read the rest in one call.
	var rest [][]byte
	var at []int
	for i, key := range keys {
		if !done[i] {
			rest, at = append(rest, key), append(at, i)
		}
	}
	if len(rest) == 0 {
		return data, found, nil
	}
	if !read {
		has, err := s.records.Has(ctx, rest)
		if err != nil {
			return nil, nil, err
		}
		for j, i := range at {
			found[i] = has[j]
		}
		return data, found, nil
	}
	values, err := s.records.Get(ctx, rest)
	if err != nil {
		return nil, nil, err
	}
	for j, i := range at {
		data[i], found[i] = values[j], values[j] != nil
	}
	return data, found, nil
}

// GetBlock reads a block's payload.
func (s *Store) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := s.lookup(ctx, []*block.BlockRef{ref}, true)
	if err != nil || !found[0] {
		return nil, false, err
	}
	return data[0], true, nil
}

// GetStoredBlock serves the block without refs because this store keeps
// block bytes without their refs.
func (s *Store) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return block.GetBlockWithoutRefs(ctx, s, ref)
}

// GetBlockExists checks whether a block is stored.
func (s *Store) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	_, found, err := s.lookup(ctx, []*block.BlockRef{ref}, false)
	return err == nil && found[0], err
}

// GetBlockExistsBatch checks each reference in one record store call.
func (s *Store) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	_, found, err := s.lookup(ctx, refs, false)
	return found, err
}

// StatBlock returns a block's payload length. A record store keeps no length
// apart from the payload, so it reads the payload.
func (s *Store) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	data, found, err := s.lookup(ctx, []*block.BlockRef{ref}, true)
	if err != nil || !found[0] {
		return nil, err
	}
	return &block.BlockStat{Ref: ref.Clone(), Size: int64(len(data[0]))}, nil
}

// RmBlock records a pending remove, committed by the next Sync or commit.
func (s *Store) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	_, err := s.add(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}})
	return err
}

// Sync commits the pending block writes and removes durably in one record
// store call, which also makes the earlier ordered commits durable. It holds
// the table's writer lock, since the table's commits must not overlap.
func (s *Store) Sync(ctx context.Context) (bool, error) {
	tx, err := s.table.NewTransaction(ctx, true)
	if err != nil {
		return false, err
	}
	defer tx.Discard()
	return true, s.commit(ctx, memtable.Snapshot{}, nil, false)
}

// blockKey returns the record key of a block reference.
func blockKey(ref *block.BlockRef) ([]byte, error) {
	if err := ref.Validate(false); err != nil {
		return nil, err
	}
	key, err := ref.MarshalKey()
	if err != nil {
		return nil, err
	}
	return append([]byte(blockPrefix), key...), nil
}
