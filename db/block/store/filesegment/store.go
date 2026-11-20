package block_store_filesegment

import (
	"context"
	"io"
	"os"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
)

// Store is a read-only block source whose blocks are file ranges. It returns a
// block only when the range re-encodes to the requested ref, so a changed file
// reads as a miss and never as wrong data. It never writes or deletes.
//
// Prototype: it embeds block.NopStoreOps for the write and root methods.
type Store struct {
	block.NopStoreOps
	idx  *Index
	xfrm block.Transformer

	// Hits counts reads served from a file.
	Hits atomic.Int64
	// StatMisses counts ranges skipped because the file's stat changed.
	StatMisses atomic.Int64
	// HashMisses counts ranges whose bytes changed under an unchanged stat.
	HashMisses atomic.Int64
}

// NewStore constructs a Store over idx that encodes with xfrm, the Space's
// block transform.
func NewStore(idx *Index, xfrm block.Transformer) *Store {
	return &Store{idx: idx, xfrm: xfrm}
}

// GetBlock regenerates the block from the first indexed range that verifies.
func (s *Store) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	for _, loc := range s.idx.Lookup(ref) {
		data, ok, err := s.regenerate(ref, loc)
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
	return &block.StoredBlock{Data: data, RefsKnown: true}, nil
}

// GetBlockExists reports an indexed range whose file stat is unchanged. It
// does not hash; a read still verifies.
func (s *Store) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	for _, loc := range s.idx.Lookup(ref) {
		if fi, err := os.Stat(loc.Path); err == nil && statMatches(fi, loc) {
			return true, nil
		}
	}
	return false, nil
}

// regenerate reads loc, encodes it, and checks it against ref.
func (s *Store) regenerate(ref *block.BlockRef, loc Location) ([]byte, bool, error) {
	// A missing file is a miss.
	f, err := os.Open(loc.Path)
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
	if !statMatches(fi, loc) {
		s.StatMisses.Add(1)
		return nil, false, nil
	}

	// Read, encode and verify; a mismatch means the bytes changed under an
	// unchanged stat.
	buf := make([]byte, loc.Length)
	if _, err := f.ReadAt(buf, loc.Offset); err != nil && err != io.EOF {
		return nil, false, err
	}
	enc := buf
	if s.xfrm != nil {
		if enc, err = s.xfrm.EncodeBlock(buf); err != nil {
			return nil, false, err
		}
	}
	if ref.VerifyData(enc, false) != nil {
		s.HashMisses.Add(1)
		return nil, false, nil
	}
	s.Hits.Add(1)
	return enc, true, nil
}

func statMatches(fi os.FileInfo, loc Location) bool {
	return fi.Size() == loc.Size && fi.ModTime().UnixNano() == loc.ModTime
}

var _ block.StoreOps = (*Store)(nil)
