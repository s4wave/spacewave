package packfile

import (
	"github.com/s4wave/spacewave/net/hash"
)

// BlockKey returns the pack index key of a block: the protobuf encoding of its
// hash. Bloom filters hold the same keys.
func BlockKey(h *hash.Hash) []byte {
	return h.MarshalDigest()
}

// ParseBlockKey parses a pack index key into the block hash it names.
func ParseBlockKey(key []byte) (*hash.Hash, error) {
	h := &hash.Hash{}
	if err := h.UnmarshalVT(key); err != nil {
		return nil, err
	}
	if err := h.Validate(); err != nil {
		return nil, err
	}
	return h, nil
}
