package hash

import (
	"bytes"
	"crypto/sha256"
	"hash"
	"slices"

	// We include sha1 for git support.
	"crypto/sha1" //nolint:gosec

	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/zeebo/blake3"
)

// ErrHashMismatch is returned when hashes mismatch.
var ErrHashMismatch = errors.New("hash mismatch")

// ErrHashTypeUnknown is returned when the hash type is unknown.
var ErrHashTypeUnknown = errors.New("unknown hash type")

// SupportedHashTypes is the list of built-in hash types.
var SupportedHashTypes = []HashType{
	HashType_HashType_SHA256,
	HashType_HashType_SHA1,
	HashType_HashType_BLAKE3,
}

// RecommendedHashType is the hash type recommended to use.
// Note: not guaranteed to stay the same between Bifrost versions.
const RecommendedHashType = HashType_HashType_BLAKE3

// UnmarshalHashJSON unmarshals a hash from json.
func UnmarshalHashJSON(data []byte) (*Hash, error) {
	h := &Hash{}
	if err := h.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	return h, nil
}

// IsNil checks if the object is nil.
func (h *Hash) IsNil() bool {
	return h == nil
}

// IsEmpty checks if the hash is empty.
func (h *Hash) IsEmpty() bool {
	return h.GetHashType() == 0 || len(h.GetHash()) == 0
}

// Clone clones the hash object.
func (h *Hash) Clone() *Hash {
	if h == nil {
		return nil
	}

	return &Hash{
		HashType: h.GetHashType(),
		Hash:     slices.Clone(h.GetHash()),
	}
}

// Validate validates the hash type.
func (h HashType) Validate() error {
	switch h {
	case HashType_HashType_UNKNOWN:
		return nil
	case HashType_HashType_SHA256:
		return nil
	case HashType_HashType_SHA1:
		return nil
	case HashType_HashType_BLAKE3:
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
	case HashType_HashType_SHA1:
		return sha1.Size //nolint:gosec
	case HashType_HashType_BLAKE3:
		return 32
	}
	return 0
}

// Sum takes the sum with the hash type.
func (h HashType) Sum(data []byte) ([]byte, error) {
	switch h {
	case HashType_HashType_SHA256:
		h := sha256.Sum256(data)
		return h[:], nil
	case HashType_HashType_SHA1:
		h := sha1.Sum(data) //nolint:gosec
		return h[:], nil
	case HashType_HashType_BLAKE3:
		h := blake3.Sum256(data)
		return h[:], nil
	default:
		return nil, errors.Errorf("hash type unknown: %v", h.String())
	}
}

// CompareHash compares two hashes.
func (h *Hash) CompareHash(other *Hash) bool {
	if other == nil && h == nil {
		return true
	}
	if h == nil || other == nil {
		return false
	}
	if h.GetHashType() != other.GetHashType() {
		return false
	}
	if len(h.GetHash()) != len(other.GetHash()) {
		return false
	}
	if !bytes.Equal(h.GetHash(), other.GetHash()) {
		return false
	}
	return true
}

// BuildHasher builds the hasher for the hash type.
func (h HashType) BuildHasher() (hash.Hash, error) {
	switch h {
	case HashType_HashType_SHA256:
		return sha256.New(), nil
	case HashType_HashType_SHA1:
		return sha1.New(), nil //nolint:gosec
	case HashType_HashType_BLAKE3:
		return blake3.New(), nil
	default:
		return nil, errors.Errorf("hash type unknown: %v", h.String())
	}
}

// VerifyData verifies data against the sum.
// Returns the hash of the data, hash type, and error
// Returns an error if failed to validate.
func (h *Hash) VerifyData(data []byte) ([]byte, error) {
	hash, err := h.GetHashType().Sum(data)
	if err != nil {
		return nil, err
	}
	if len(hash) != len(h.GetHash()) {
		return hash, ErrHashMismatch
	}
	if !bytes.Equal(hash, h.GetHash()) {
		return hash, ErrHashMismatch
	}
	return hash, nil
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
	dat, err := h.MarshalVT()
	if err != nil {
		return ""
	}
	return b58.Encode(dat)
}

// MarshalDigest marshals the hash to a binary slice.
func (h *Hash) MarshalDigest() []byte {
	if h == nil {
		return nil
	}
	dat, _ := h.MarshalVT()
	return dat
}

// ParseFromB58 parses the object ref from a base58 string.
func (h *Hash) ParseFromB58(ref string) error {
	dat, err := b58.Decode(ref)
	if err != nil {
		return err
	}
	return h.UnmarshalVT(dat)
}
