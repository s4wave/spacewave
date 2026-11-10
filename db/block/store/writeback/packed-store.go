package block_store_writeback

import (
	"context"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/s4wave/spacewave/net/hash"
)

// PackedStore keeps a block store's new values in a packahead Log, whose
// finalized packs upload as they are, and lands only their child lists in
// local as external entries, so the Volume holds each new block as it holds
// an evicted one. Reads take a value from the Log first, then from
// local, which still holds the values written before the Log and the cache
// fills.
//
// A write appends its values to the Log, which returns once they are durable,
// and then lands the entries in local. A crash between the two leaves values
// no child list names, which upload as garbage and lose nothing; the other
// order could leave a child list naming bytes the disk lost.
//
// With divert unset, writes go to local as they did before the Log, and the
// Log's values stay readable and draining: the switch back to the old path.
type PackedStore struct {
	// log holds the new values.
	log *packahead.Log
	// local is the store's bucket on the Volume.
	local block.StoreOps
	// divert sends new values to the Log; unset, writes go to local.
	divert bool
	// counts counts the writes, shared by the read scopes.
	counts *packedCounts
}

// packedCounts counts a PackedStore's writes.
type packedCounts struct {
	diverted, divertedBytes, held, fills atomic.Uint64
}

// PackedStats counts a PackedStore's writes since it was built.
type PackedStats struct {
	// Diverted counts the values appended to the Log.
	Diverted uint64
	// DivertedBytes is their encoded size.
	DivertedBytes uint64
	// Held counts the diverted values the Log held already, so appended
	// nothing.
	Held uint64
	// Fills counts the cache fills sent to the local store while diverting,
	// which the upload markers, not the Log, upload.
	Fills uint64
}

// NewPackedStore builds a PackedStore reading log before local, and keeping
// new values in log, landing their child lists in local, when
// divert is set.
func NewPackedStore(log *packahead.Log, local block.StoreOps, divert bool) *PackedStore {
	return &PackedStore{log: log, local: local, divert: divert, counts: &packedCounts{}}
}

// GetDivert reports whether the store diverts new values to the Log.
func (s *PackedStore) GetDivert() bool {
	return s.divert
}

// GetStats returns the store's write counts.
func (s *PackedStore) GetStats() PackedStats {
	return PackedStats{
		Diverted:      s.counts.diverted.Load(),
		DivertedBytes: s.counts.divertedBytes.Load(),
		Held:          s.counts.held.Load(),
		Fills:         s.counts.fills.Load(),
	}
}

// GetStoreParts returns the local store.
func (s *PackedStore) GetStoreParts() []block.StorePart {
	return []block.StorePart{{Label: "local", Store: s.local}}
}

// GetHashType returns the local store's hash type.
func (s *PackedStore) GetHashType() hash.HashType {
	return s.local.GetHashType()
}

// GetSupportedFeatures returns the local store's features.
func (s *PackedStore) GetSupportedFeatures() block.StoreFeature {
	return s.local.GetSupportedFeatures()
}

// BeginReadOperation opens a read scope on the local store; the Log needs
// none.
func (s *PackedStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scope, release, err := s.local.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &PackedStore{log: s.log, local: scope, divert: s.divert, counts: s.counts}, release, nil
}

// PutBlock stores a block as PutBlockBatch does. A cache fill, a copy of a
// block another store holds, goes to the local store, so it never uploads.
func (s *PackedStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Send a cache fill, or any write while not diverting, to the local store.
	if opts.GetCacheFill() || !s.divert {
		if s.divert {
			s.counts.fills.Add(1)
		}
		return s.local.PutBlock(ctx, data, opts)
	}

	// Name the block as the local store would.
	opts = opts.CloneVT()
	if opts == nil {
		opts = &block.PutOpts{}
	}
	opts.HashType = opts.SelectHashType(s.GetHashType())
	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	if forced := opts.GetForceBlockRef(); !forced.GetEmpty() && !ref.EqualsRef(forced) {
		return ref, false, block.ErrBlockRefMismatch
	}

	// Store it as a batch of one.
	entry := &block.PutBatchEntry{Ref: ref, Data: data, Refs: opts.GetRefs(), RefsKnown: opts.GetRefsKnown()}
	existed, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{entry})
	if err != nil {
		return ref, false, err
	}

	// Make the local store durable when the write asks.
	if opts.GetSync() {
		if _, err := s.local.Sync(ctx); err != nil {
			return ref, existed[0], err
		}
	}
	return ref, existed[0], nil
}

// PutBlockBatch appends the batch's values to the Log, then lands the batch
// in the local store as external entries, and reports whether each block was
// stored before, in the Log or the local store.
func (s *PackedStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Write to the local store while not diverting.
	if !s.divert {
		return s.local.PutBlockBatch(ctx, entries)
	}

	// Append the values, then land the entries.
	landed, held, err := s.appendValues(ctx, entries)
	if err != nil {
		return nil, err
	}
	existed, err := s.local.PutBlockBatch(ctx, landed)
	if err != nil {
		return nil, err
	}
	s.forgetTombstones(entries)
	for i := range existed {
		existed[i] = existed[i] || held[i]
	}
	return existed, nil
}

// appendValues appends the values of entries the Log does not hold, and
// returns external copies of the entries to land, and whether the Log held
// each block before. Tombstones pass through. A block whose bytes the local
// store holds already is appended too: asking would cost a read of the local
// store per block, the read this store exists to remove, and the copy holds
// the same bytes.
func (s *PackedStore) appendValues(ctx context.Context, entries []*block.PutBatchEntry) ([]*block.PutBatchEntry, []bool, error) {
	// Encode each new value under its pack key, naming unnamed entries.
	landed := make([]*block.PutBatchEntry, len(entries))
	held := make([]bool, len(entries))
	var values []packahead.Block
	for i, entry := range entries {
		landed[i] = entry
		if entry == nil || entry.Tombstone || entry.External {
			continue
		}
		copied := *entry
		if copied.Ref.GetEmpty() {
			ref, err := block.BuildBlockRef(copied.Data, &block.PutOpts{HashType: s.GetHashType()})
			if err != nil {
				return nil, nil, err
			}
			copied.Ref = ref
		}
		copied.External = true
		landed[i] = &copied
		key := packfile.BlockKey(copied.Ref.GetHash())
		if held[i] = s.log.Has(key); held[i] {
			s.counts.held.Add(1)
			continue
		}
		value, err := block.EncodeBlockObject(copied.Data, copied.Refs, copied.RefsKnown)
		if err != nil {
			return nil, nil, err
		}
		values = append(values, packahead.Block{Key: key, Value: value})
	}

	// Make the values durable before any child list names them.
	if len(values) != 0 {
		if err := s.log.Append(ctx, values); err != nil {
			return nil, nil, errors.Wrap(err, "append values")
		}
		var size int
		for _, v := range values {
			size += len(v.Value)
		}
		s.counts.diverted.Add(uint64(len(values)))
		s.counts.divertedBytes.Add(uint64(size)) //nolint:gosec // a length is not negative.
	}
	return landed, held, nil
}

// forgetTombstones forgets the Log's copies of the tombstoned blocks, once
// the local store removed them.
func (s *PackedStore) forgetTombstones(entries []*block.PutBatchEntry) {
	for _, entry := range entries {
		if entry != nil && entry.Tombstone && !entry.Ref.GetEmpty() {
			s.log.Remove(packfile.BlockKey(entry.Ref.GetHash()))
		}
	}
}

// GetBlock returns the block's bytes from the Log or the local store.
func (s *PackedStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return nil, false, err
	}
	return stored.Data, true, nil
}

// GetStoredBlock returns the block with its refs from the Log, checking its
// bytes against ref, or from the local store.
func (s *PackedStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Read the Log's copy and check it.
	if !ref.GetEmpty() {
		value, ok, err := s.log.Get(packfile.BlockKey(ref.GetHash()))
		if err != nil {
			return nil, err
		}
		if ok {
			stored, err := block.DecodeBlockObject(value)
			if err != nil {
				return nil, errors.Wrap(err, ref.MarshalString())
			}
			if err := ref.VerifyData(stored.Data, false); err != nil {
				return nil, errors.Wrap(err, ref.MarshalString())
			}
			return stored, nil
		}
	}

	// Read the local store's.
	return s.local.GetStoredBlock(ctx, ref)
}

// GetBlockExists reports whether the Log or the local store holds the block.
func (s *PackedStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	if !ref.GetEmpty() && s.log.Has(packfile.BlockKey(ref.GetHash())) {
		return true, nil
	}
	return s.local.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch reports, for each block, whether the Log or the local
// store holds it.
func (s *PackedStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	exists, err := s.local.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, err
	}
	for i, ref := range refs {
		if !exists[i] && !ref.GetEmpty() {
			exists[i] = s.log.Has(packfile.BlockKey(ref.GetHash()))
		}
	}
	return exists, nil
}

// StatBlock returns the block's size from the Log or the local store.
func (s *PackedStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	// Stat the Log's copy by decoding it.
	if !ref.GetEmpty() {
		value, ok, err := s.log.Get(packfile.BlockKey(ref.GetHash()))
		if err != nil {
			return nil, err
		}
		if ok {
			stored, err := block.DecodeBlockObject(value)
			if err != nil {
				return nil, errors.Wrap(err, ref.MarshalString())
			}
			return &block.BlockStat{Ref: ref, Size: int64(len(stored.Data))}, nil
		}
	}

	// Stat the local store's.
	return s.local.StatBlock(ctx, ref)
}

// RmBlock removes the block from the local store, which keeps its child list,
// and forgets the Log's copy.
func (s *PackedStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if err := s.local.RmBlock(ctx, ref); err != nil {
		return err
	}
	if !ref.GetEmpty() {
		s.log.Remove(packfile.BlockKey(ref.GetHash()))
	}
	return nil
}

// Sync makes the local store durable; every Log append is durable when it
// returns.
func (s *PackedStore) Sync(ctx context.Context) (bool, error) {
	return s.local.Sync(ctx)
}

// InvalidateDecodedBlockRef forwards the decoded-cache invalidation of a
// removed block to the local store, so the MarkingStore above still reaches
// it.
func (s *PackedStore) InvalidateDecodedBlockRef(ctx context.Context, ref *block.BlockRef) {
	if invalidator, ok := s.local.(decodedBlockRefInvalidator); ok {
		invalidator.InvalidateDecodedBlockRef(ctx, ref)
	}
}

// _ is a type assertion
var (
	_ block.StoreOps             = (*PackedStore)(nil)
	_ decodedBlockRefInvalidator = (*PackedStore)(nil)
)
