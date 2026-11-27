package writer

import (
	"bytes"
	"crypto/sha256"
	gohash "hash"
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
	// ValueOrderPolicy records how physical kvfile values were ordered.
	ValueOrderPolicy string
}

// BlockIterator yields each block to pack with its hash. It returns a nil
// hash when exhausted.
type BlockIterator func() (h *hash.Hash, blk *block.StoredBlock, err error)

// PackBlocks packs blocks from the iterator into a kvfile, in the order it
// yields them, and computes a bloom filter sized to the packed block count.
// Each value is the encoded block.BlockObject of the block's bytes and refs. A
// block without known refs is stored marked so, and reads back with its refs
// unknown.
func PackBlocks(w io.Writer, iter BlockIterator) (*PackResult, error) {
	// Encode each block into the pack as the iterator yields it.
	p := newPacker(w)
	for {
		h, blk, err := iter()
		if err != nil {
			return nil, errors.Wrap(err, "iterating blocks")
		}
		if h == nil {
			break
		}
		if err := p.add(h, blk); err != nil {
			return nil, err
		}
	}
	return p.finish(ValueOrderIterator)
}

// packer writes block values into a kvfile and keeps the keys for its bloom
// filter and identity digest.
type packer struct {
	// digest hashes the kvfile bytes as they are written.
	digest gohash.Hash
	// kvw writes the values and, on finish, the index.
	kvw *kvfile.Writer
	// keys lists the written block keys in write order.
	keys [][]byte
}

// newPacker starts a kvfile written to w.
func newPacker(w io.Writer) *packer {
	h := sha256.New()
	return &packer{digest: h, kvw: kvfile.NewWriter(io.MultiWriter(w, h))}
}

// add writes blk as the next value, under h's block key.
func (p *packer) add(h *hash.Hash, blk *block.StoredBlock) error {
	// Encode the block and write it, keeping its key.
	key := packfile.BlockKey(h)
	value, err := block.EncodeBlockObject(blk.Data, blk.Refs, blk.GetRefsSource().Recorded())
	if err != nil {
		return errors.Wrap(err, "encoding block value")
	}
	if err := p.kvw.WriteValue(key, bytes.NewReader(value)); err != nil {
		return errors.Wrap(err, "writing block to kvfile")
	}
	p.keys = append(p.keys, bytes.Clone(key))
	return nil
}

// finish writes the kvfile index and reports the pack, its values laid out by
// valueOrder.
func (p *packer) finish(valueOrder string) (*PackResult, error) {
	// Finish the kvfile index before reporting the completed pack.
	if err := p.kvw.Close(); err != nil {
		return nil, errors.Wrap(err, "closing kvfile writer")
	}

	// Build the bloom filter from the packed block keys.
	policy := DefaultPolicy()
	count := uint64(len(p.keys))
	bf := policy.NewBloomFilter(count)
	for _, key := range p.keys {
		bf.Add(key)
	}
	bloomBytes, err := bloom.NewBloom(bf).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshaling bloom filter")
	}

	return &PackResult{
		BloomFilter:      bloomBytes,
		BlockCount:       count,
		BytesWritten:     p.kvw.GetPos(),
		SortedKeyDigest:  DigestSortedKeys(p.keys),
		PackBytesDigest:  p.digest.Sum(nil),
		PolicyTag:        PolicyTag(policy),
		ValueOrderPolicy: valueOrder,
	}, nil
}
