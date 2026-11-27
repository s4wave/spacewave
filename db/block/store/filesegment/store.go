package block_store_filesegment

import (
	"context"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"
)

// Store is the FileSegmentStore: a read-only block source whose blocks are
// file ranges recorded in a SegmentIndex. It returns a block only when the
// range re-encodes to the requested ref, so a changed file reads as a miss and
// never as wrong data. A range whose bytes changed under an unchanged stat
// drops its path's ranges from the index and is reported to the watchers of
// hash misses, which link the file again. It never writes or deletes a block
// or a file. Its write methods return block_store.ErrReadOnly.
//
// It reads nothing until SetTransformer gives it the Space's transform, which
// the World holds and the session lock keeps from rest.
type Store struct {
	// idx is the SegmentIndex.
	idx *Index
	// xfrm holds the Space's transform, or nil before SetTransformer.
	xfrm atomic.Pointer[transformer]
	// bufs pools the read buffers. Each pool entry holds a *[]byte.
	bufs sync.Pool

	// Hits counts reads served from a file.
	Hits atomic.Int64
	// StatMisses counts ranges skipped because the file's stat changed.
	StatMisses atomic.Int64
	// HashMisses counts ranges whose bytes changed under an unchanged stat.
	HashMisses atomic.Int64

	// watchMtx guards watchers and nextWatcher.
	watchMtx sync.Mutex
	// watchers receive the path of each hash miss, by registration.
	watchers map[uint64]func(path string)
	// nextWatcher is the next registration.
	nextWatcher uint64
}

// transformer boxes a block.Transformer for an atomic pointer.
type transformer struct {
	// Transformer is the Space's transform.
	block.Transformer
}

// NewStore constructs a Store over idx.
func NewStore(idx *Index) *Store {
	return &Store{idx: idx}
}

// GetIndex returns the SegmentIndex the store reads.
func (s *Store) GetIndex() *Index {
	return s.idx
}

// SetTransformer sets the Space's transform, with which every block of the
// Space is encoded.
func (s *Store) SetTransformer(xfrm block.Transformer) {
	s.xfrm.Store(&transformer{Transformer: xfrm})
}

// GetHashType returns 0: the store takes no writes.
func (s *Store) GetHashType() hash.HashType {
	return 0
}

// GetSupportedFeatures reports no native features.
func (s *Store) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeature_STORE_FEATURE_UNKNOWN
}

// BeginReadOperation returns the store itself: reads hold no scope.
func (s *Store) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// GetRoot reports that no name is held.
func (s *Store) GetRoot(context.Context, string) (*block.BlockRef, error) {
	return nil, nil
}

// ListRoots reports no roots.
func (s *Store) ListRoots(context.Context, string, func(block.NamedRoot) error) error {
	return nil
}

// SetRoots rejects the write.
func (s *Store) SetRoots(context.Context, []*block.PutBatchEntry, []block.NamedRoot) error {
	return block_store.ErrReadOnly
}

// PutBlock rejects the write.
func (s *Store) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

// PutBlockBatch rejects the writes.
func (s *Store) PutBlockBatch(context.Context, []*block.PutBatchEntry) ([]bool, error) {
	return nil, block_store.ErrReadOnly
}

// RmBlock rejects the delete.
func (s *Store) RmBlock(context.Context, *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// Sync has nothing to fence: the store accepted no writes.
func (s *Store) Sync(context.Context) (bool, error) {
	return false, nil
}

// GetBlock regenerates the block from the first indexed range that verifies.
func (s *Store) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Read nothing without the Space's transform.
	xfrm := s.xfrm.Load()
	if xfrm == nil {
		return nil, false, nil
	}

	// Try each range until one verifies.
	segs, err := s.idx.Lookup(ctx, ref)
	if err != nil {
		return nil, false, err
	}
	for _, seg := range segs {
		data, ok, err := s.regenerate(ctx, xfrm, ref, seg)
		if err != nil || ok {
			return data, ok, err
		}
	}
	return nil, false, nil
}

// GetStoredBlock returns the regenerated block. A file range is always a chunk
// data block, a leaf, so its refs are known and empty.
func (s *Store) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	data, ok, err := s.GetBlock(ctx, ref)
	if err != nil || !ok {
		return nil, err
	}
	return &block.StoredBlock{Data: data, RefsSource: block.RefsAsserted}, nil
}

// GetBlockExists reports an indexed range whose file stat is unchanged. It
// does not hash, but a read still verifies.
func (s *Store) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	seg, err := s.statSegment(ctx, ref)
	return seg != nil, err
}

// statSegment returns the first indexed range of ref whose file stat is
// unchanged, or nil. It reads no file contents.
func (s *Store) statSegment(ctx context.Context, ref *block.BlockRef) (*Segment, error) {
	// Report nothing the store could not read.
	if s.xfrm.Load() == nil {
		return nil, nil
	}

	// Check the stat of each range.
	segs, err := s.idx.Lookup(ctx, ref)
	if err != nil {
		return nil, err
	}
	for _, seg := range segs {
		if fi, err := os.Stat(seg.GetPath()); err == nil && statMatches(fi, seg) {
			return seg, nil
		}
	}
	return nil, nil
}

// GetBlockExistsBatch checks each ref in turn.
func (s *Store) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := s.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

// StatBlock returns the encoded size recorded for a range whose file stat is
// unchanged, or nil. Like GetBlockExists it reads, encodes and hashes nothing,
// but a read still verifies.
func (s *Store) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	seg, err := s.statSegment(ctx, ref)
	if err != nil || seg == nil {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: seg.GetBlockSize()}, nil
}

// GetCopyTier returns the store as the file tier of a copy chain, whose stored
// blocks are marked Linked. Only a copy chain that reads every Volume tier
// before the file tier may use the file tier, so a Linked block is one no
// Volume tier holds.
func (s *Store) GetCopyTier() block.StoreOps {
	return copyTier{Store: s}
}

// copyTier is the Store as a copy chain's file tier.
type copyTier struct {
	*Store
}

// BeginReadOperation returns the tier itself: reads hold no scope.
func (t copyTier) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return t, func() {}, nil
}

// GetStoredBlock returns the regenerated block, marked Linked.
func (t copyTier) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := t.Store.GetStoredBlock(ctx, ref)
	if stored != nil {
		stored.Linked = true
	}
	return stored, err
}

// WatchHashMisses calls cb with the path of each file whose bytes changed
// under an unchanged stat, after the store dropped the path's ranges, until
// the returned release is called. cb runs on the read that found the miss, so
// it must return at once.
func (s *Store) WatchHashMisses(cb func(path string)) func() {
	// Register cb under the next ID, which the release removes.
	s.watchMtx.Lock()
	defer s.watchMtx.Unlock()
	if s.watchers == nil {
		s.watchers = make(map[uint64]func(string))
	}
	id := s.nextWatcher
	s.nextWatcher++
	s.watchers[id] = cb
	return func() {
		s.watchMtx.Lock()
		delete(s.watchers, id)
		s.watchMtx.Unlock()
	}
}

// dropStale drops the ranges of path, whose bytes changed under an unchanged
// stat, and reports the path to the watchers so its file is linked again.
//
// A failed drop leaves ranges that keep missing until the relink replaces
// them, so the read reports its miss either way and the error goes nowhere.
// A drop that races the relink of the same path can remove the new ranges.
// The relink this drop reports runs after the drop and restores them.
func (s *Store) dropStale(ctx context.Context, path string) {
	// Drop the path's ranges.
	_ = s.idx.DropPath(ctx, path)

	// Report the path.
	s.watchMtx.Lock()
	defer s.watchMtx.Unlock()
	for _, cb := range s.watchers {
		cb(path)
	}
}

// regenerate reads seg, encodes it with xfrm, and checks it against ref. A
// range whose bytes changed under an unchanged stat drops its path.
func (s *Store) regenerate(ctx context.Context, xfrm *transformer, ref *block.BlockRef, seg *Segment) ([]byte, bool, error) {
	// A missing file is a miss.
	f, err := os.Open(seg.GetPath())
	if err != nil {
		if os.IsNotExist(err) {
			s.StatMisses.Add(1)
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()

	// A changed stat is a fast miss.
	fi, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	if !statMatches(fi, seg) {
		s.StatMisses.Add(1)
		return nil, false, nil
	}

	// Read the range into a pooled buffer, which the encode only reads unless
	// the transform has no steps and returns it as the block.
	bp := s.getBuf(seg.GetLength())
	buf := *bp
	if _, err := f.ReadAt(buf, seg.GetOffset()); err != nil && err != io.EOF {
		s.bufs.Put(bp)
		return nil, false, err
	}
	enc, err := xfrm.EncodeBlock(buf)
	if err != nil {
		s.bufs.Put(bp)
		return nil, false, err
	}
	if len(enc) == 0 || &enc[0] != &buf[0] {
		s.bufs.Put(bp)
	}

	// Verify the bytes. A mismatch means the bytes changed under an
	// unchanged stat.
	if ref.VerifyData(enc, false) != nil {
		s.HashMisses.Add(1)
		s.dropStale(ctx, seg.GetPath())
		return nil, false, nil
	}
	s.Hits.Add(1)
	return enc, true, nil
}

// getBuf returns a pooled buffer of length n.
func (s *Store) getBuf(n int64) *[]byte {
	bp, _ := s.bufs.Get().(*[]byte)
	if bp == nil || int64(cap(*bp)) < n {
		b := make([]byte, n)
		return &b
	}
	*bp = (*bp)[:n]
	return bp
}

// statMatches reports whether fi is the stat seg was linked under.
func statMatches(fi os.FileInfo, seg *Segment) bool {
	return fi.Size() == seg.GetSize() && fi.ModTime().UnixNano() == seg.GetModTime()
}

// _ is a type assertion
var (
	_ block.StoreOps = (*Store)(nil)
	_ block.StoreOps = copyTier{}
)
