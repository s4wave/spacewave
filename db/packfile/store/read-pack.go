package store

import (
	"bytes"
	"context"
	"slices"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// PackBlock is one verified block read from a pack.
type PackBlock struct {
	// Hash is the block hash the pack stores the block under.
	Hash *hash.Hash
	// Block is the block data with its refs.
	Block *block.StoredBlock
}

// ReadPackBlocks fetches a whole pack with one transport read and returns its
// blocks in the order the pack stores their values. Every value is checked
// against its key hash, as a served read is. The read uses a fresh reader
// instead of the pack's engine, so it neither evicts nor fills the resident
// spans that serve block reads.
func (s *PackfileStore) ReadPackBlocks(ctx context.Context, packID string, size int64) ([]PackBlock, error) {
	rd, err := s.opener(packID, size)
	if err != nil {
		return nil, errors.Wrapf(err, "open packfile %s", packID)
	}
	defer rd.Close()

	data, err := rd.transport.Fetch(ctx, 0, int(size))
	if err != nil {
		return nil, errors.Wrapf(err, "fetch packfile %s", packID)
	}
	if int64(len(data)) != size {
		return nil, errors.Errorf("packfile %s: read %d of %d bytes", packID, len(data), size)
	}
	kv, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		return nil, errors.Wrapf(err, "read packfile %s index", packID)
	}

	type located struct {
		entry *kvfile.IndexEntry
		idx   int
		off   int64
	}
	var entries []located
	err = kv.ScanPrefixEntries(nil, func(entry *kvfile.IndexEntry, idx int) error {
		off, _, err := kv.GetValuePositionWithEntry(entry, idx)
		entries = append(entries, located{entry: entry, idx: idx, off: off})
		return err
	})
	if err != nil {
		return nil, errors.Wrapf(err, "scan packfile %s index", packID)
	}
	slices.SortFunc(entries, func(a, b located) int {
		return int(a.off - b.off)
	})

	blocks := make([]PackBlock, 0, len(entries))
	for _, e := range entries {
		h, err := packfile.ParseBlockKey(e.entry.GetKey())
		if err != nil {
			return nil, errors.Wrapf(err, "packfile %s key", packID)
		}
		value, err := kv.GetWithEntry(e.entry, e.idx)
		if err != nil {
			return nil, errors.Wrapf(err, "packfile %s value", packID)
		}
		stored, err := decodeVerifiedBlock(block.NewBlockRef(h), value)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, PackBlock{Hash: h, Block: stored})
	}
	return blocks, nil
}
