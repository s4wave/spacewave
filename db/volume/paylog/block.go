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

// PutBlock appends the payload to the open segment. The block is readable at
// once and durable after the next Sync or index commit.
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

	// Append the payload unless the block is already stored.
	written, err := s.append(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data}})
	if err != nil {
		return ref, false, err
	}
	if opts.GetSync() {
		_, err = s.Sync(ctx)
	}
	return ref, written == 0, err
}

// PutBlockBatch verifies every entry, then appends the new payloads in one
// device call and records the tombstones as removes.
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
	_, err := s.append(ctx, entries)
	return err
}

// append writes the payloads of entries not yet stored in one device call and
// adds their pending locations, and adds a pending remove for each tombstone.
// It returns the number of payloads written.
func (s *Store) append(ctx context.Context, entries []*block.PutBatchEntry) (int, error) {
	// Resolve keys and find the stored blocks in one index view.
	keys := make([]string, len(entries))
	stored := make([]bool, len(entries))
	for i, entry := range entries {
		key, err := blockKey(entry.Ref)
		if err != nil {
			return 0, err
		}
		keys[i] = key
	}
	err := s.view(ctx, func(tx kvtx.Tx) error {
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
		return 0, err
	}

	// Lay out the new payloads and issue them before recording them.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	var writes []device.Write
	added := make(map[string]*pendingBlock)
	for i, entry := range entries {
		key := keys[i]
		if entry.Tombstone {
			added[key] = &pendingBlock{}
			continue
		}
		p := added[key]
		if p == nil {
			p = s.pending[key]
		}
		if (p != nil && p.loc != nil) || (p == nil && stored[i]) {
			continue
		}
		if s.offset != 0 && s.offset+int64(len(entry.Data)) > segmentSize {
			s.segment++
			s.offset = 0
		}
		loc := &location{segment: s.segment, offset: s.offset, length: int64(len(entry.Data))}
		writes = append(writes, device.Write{Name: segmentName(loc.segment), Offset: loc.offset, Data: entry.Data})
		s.offset += loc.length
		added[key] = &pendingBlock{loc: loc}
	}
	if len(writes) != 0 {
		if err := s.dev.Write(ctx, writes, false); err != nil {
			return 0, err
		}
	}
	maps.Copy(s.pending, added)
	return len(writes), nil
}

// locate returns the locations of refs, nil for an absent block. It reads
// pending entries before opening the index view, so an entry published in
// between is found in the view.
func (s *Store) locate(ctx context.Context, refs []*block.BlockRef) ([]*location, error) {
	// Resolve keys and answer from pending entries.
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
	s.mtx.Lock()
	for i, key := range keys {
		if p := s.pending[key]; p != nil && !found[i] {
			locs[i], found[i] = p.loc, true
		}
	}
	s.mtx.Unlock()

	// Read the rest from one index view.
	err := s.view(ctx, func(tx kvtx.Tx) error {
		for i, key := range keys {
			if found[i] {
				continue
			}
			v, ok, err := tx.Get(ctx, []byte(key))
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			loc, err := unmarshalLocation(v)
			if err != nil {
				return err
			}
			locs[i] = loc
		}
		return nil
	})
	return locs, err
}

// GetBlock reads a block's payload.
func (s *Store) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	locs, err := s.locate(ctx, []*block.BlockRef{ref})
	if err != nil || locs[0] == nil {
		return nil, false, err
	}
	loc := locs[0]
	data := make([]byte, loc.length)
	read := []device.Read{{Name: segmentName(loc.segment), Offset: loc.offset, Data: data}}
	if err := s.dev.Read(ctx, read); err != nil {
		return nil, false, errors.Wrap(err, "read block")
	}
	return data, true, nil
}

// GetBlockExists checks whether a block is stored.
func (s *Store) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	locs, err := s.locate(ctx, []*block.BlockRef{ref})
	return err == nil && locs[0] != nil, err
}

// GetBlockExistsBatch checks each reference from one index view.
func (s *Store) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	locs, err := s.locate(ctx, refs)
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
func (s *Store) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	locs, err := s.locate(ctx, []*block.BlockRef{ref})
	if err != nil || locs[0] == nil {
		return nil, err
	}
	return &block.BlockStat{Ref: ref.Clone(), Size: locs[0].length}, nil
}

// RmBlock records a pending remove, published by the next Sync or index
// commit.
func (s *Store) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	_, err := s.append(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}})
	return err
}

// Sync publishes every pending block write and remove in one index commit,
// which also makes the earlier ordered commits durable. With nothing pending
// it flushes the device instead, which costs nothing when nothing is dirty.
func (s *Store) Sync(ctx context.Context) (bool, error) {
	s.mtx.Lock()
	idle := len(s.pending) == 0
	s.mtx.Unlock()
	if idle {
		return true, s.dev.Write(ctx, nil, true)
	}
	return true, s.update(ctx, false, nil)
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
