package hash

import (
	"crypto/sha256"
	"github.com/golang/protobuf/proto"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
)

// Validate validates the hash type.
func (h HashType) Validate() error {
	switch h {
	case HashType_HashType_UNKNOWN:
		return nil
	case HashType_HashType_SHA256:
		return nil
	default:
		return errors.Errorf("hash type unknown: %v", h.String())
	}
}

// GetHashLen returns the hash length.
func (h HashType) GetHashLen() int {
	switch h {
	case HashType_HashType_SHA256:
		return sha256.Size
	}
	return 0
}

// Sum takes the sum with the hash type.
func (h HashType) Sum(data []byte) ([]byte, error) {
	switch h {
	case HashType_HashType_SHA256:
		h := sha256.Sum256(data)
		return h[:], nil
	default:
		return nil, errors.Errorf("hash type unknown: %v", h.String())
	}
}

// NewHash constructs a new hash object.
func NewHash(ht HashType, h []byte) *Hash {
	return &Hash{HashType: ht, Hash: h}
}

// Sum constructs a hash type by summing an object.
func Sum(ht HashType, data []byte) (*Hash, error) {
	h, err := ht.Sum(data)
	if err != nil {
		return nil, err
	}
	return NewHash(ht, h), nil
}

// Validate validates the hash.
func (h *Hash) Validate() error {
	if err := h.GetHashType().Validate(); err != nil {
		return err
	}
	if ehl := h.GetHashType().GetHashLen(); len(h.GetHash()) != ehl {
		return errors.Errorf("expected hash length %d != %d", ehl, len(h.GetHash()))
	}
	return nil
}

// MarshalString marshals the hash to a string.
func (h *Hash) MarshalString() string {
	if h == nil {
		return ""
	}
	dat, err := proto.Marshal(h)
	if err != nil {
		return ""
	}
	return b58.Encode(dat)
}
