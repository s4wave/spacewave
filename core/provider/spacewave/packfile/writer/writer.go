package writer

import (
	"bytes"
	"io"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/bloom"
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
}

// BlockIterator yields hash/data pairs for packing.
type BlockIterator func() (h *hash.Hash, data []byte, err error)

// PackBlocks packs blocks from the iterator into a kvfile and computes a bloom
// filter. The iterator should return nil hash when exhausted. Returns the pack
// result or an error.
func PackBlocks(w io.Writer, iter BlockIterator) (*PackResult, error) {
	kvw := kvfile.NewWriter(w)

	bf := DefaultPolicy().NewBloomFilter()

	var count uint64
	for {
		h, data, err := iter()
		if err != nil {
			return nil, errors.Wrap(err, "iterating blocks")
		}
		if h == nil {
			break
		}

		key := []byte(h.MarshalString())
		if err := kvw.WriteValue(key, bytes.NewReader(data)); err != nil {
			return nil, errors.Wrap(err, "writing block to kvfile")
		}
		bf.Add(key)
		count++
	}

	if err := kvw.Close(); err != nil {
		return nil, errors.Wrap(err, "closing kvfile writer")
	}

	bloomProto := bloom.NewBloom(bf)
	bloomBytes, err := bloomProto.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshaling bloom filter")
	}

	return &PackResult{
		BloomFilter:  bloomBytes,
		BlockCount:   count,
		BytesWritten: kvw.GetPos(),
	}, nil
}
