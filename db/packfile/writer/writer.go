package writer

import (
	"bytes"
	"crypto/sha256"
	"io"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// PackResult contains the result of packing blocks into a kvfile.
type PackResult struct {
	// BloomFilter is the serialized bloom filter bytes.
	BloomFilter []byte
	// BlockCount is the number of blocks packed.
	BlockCount uint64
	// BytesWritten is the total bytes written.
	BytesWritten uint64
	// SortedKeyDigest is the v1 digest of the sorted block key set.
	SortedKeyDigest []byte
	// PackBytesDigest is the SHA-256 digest of the kvfile bytes.
	PackBytesDigest []byte
	// PolicyTag is the pack construction policy tag used for v1 identity.
	PolicyTag string
	// ValueOrderPolicy names how physical kvfile values were ordered.
	ValueOrderPolicy string
}

// BlockIterator yields each block to pack with its hash. It returns a nil
// hash when exhausted.
type BlockIterator func() (h *hash.Hash, blk *block.StoredBlock, err error)

// PackBlocks packs blocks from the iterator into a kvfile and computes a bloom
// filter sized to the packed block count. Each value is the encoded
// block.BlockObject of the block's bytes and refs, so every block must carry
// known refs.
func PackBlocks(w io.Writer, iter BlockIterator) (*PackResult, error) {
	packHash := sha256.New()
	kvw := kvfile.NewWriter(io.MultiWriter(w, packHash))

	var keys [][]byte
	for {
		h, blk, err := iter()
		if err != nil {
			return nil, errors.Wrap(err, "iterating blocks")
		}
		if h == nil {
			break
		}

		key := packfile.BlockKey(h)
		if !blk.GetRefsKnown() {
			return nil, errors.Wrapf(block.ErrRefsUnknown, "packing block %s", h.MarshalString())
		}
		value, err := block.EncodeBlockObject(blk.Data, blk.Refs)
		if err != nil {
			return nil, errors.Wrap(err, "encoding block value")
		}
		if err := kvw.WriteValue(key, bytes.NewReader(value)); err != nil {
			return nil, errors.Wrap(err, "writing block to kvfile")
		}
		keys = append(keys, bytes.Clone(key))
	}

	if err := kvw.Close(); err != nil {
		return nil, errors.Wrap(err, "closing kvfile writer")
	}

	policy := DefaultPolicy()
	count := uint64(len(keys))
	bf := policy.NewBloomFilter(count)
	for _, key := range keys {
		bf.Add(key)
	}
	bloomBytes, err := bloom.NewBloom(bf).MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshaling bloom filter")
	}

	return &PackResult{
		BloomFilter:      bloomBytes,
		BlockCount:       count,
		BytesWritten:     kvw.GetPos(),
		SortedKeyDigest:  DigestSortedKeys(keys),
		PackBytesDigest:  packHash.Sum(nil),
		PolicyTag:        PolicyTag(policy),
		ValueOrderPolicy: ValueOrderIterator,
	}, nil
}
