package git_block

import (
	"github.com/aperturerobotics/bifrost/hash"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"
)

// GitHashType is the hash type used in Git.
const GitHashType = hash.HashType_HashType_SHA1

// NewHash builds a new hash from a plumbing.Hash.
//
// Returns nil if the hash is empty.
func NewHash(pt plumbing.Hash) (*hash.Hash, error) {
	if pt == plumbing.ZeroHash {
		return nil, nil
	}

	// expect sha1 hash only (as of 01/2021)
	if len(pt) != 20 {
		return nil, errors.Errorf("unexpected hash length: %d", len(pt))
	}

	dat := make([]byte, len(pt))
	copy(dat, pt[:])
	return hash.NewHash(GitHashType, dat), nil
}

// FromHash converts a hash into a plumbing.Hash.
func FromHash(h *hash.Hash) (plumbing.Hash, error) {
	var out plumbing.Hash
	if err := ValidateHash(h); err != nil {
		return out, err
	}
	copy(out[:], h.GetHash())
	return out, nil
}

// NewHashSet constructs a list of hashes from a input plumbing hash set.
func NewHashSet(hashes []plumbing.Hash) ([]*hash.Hash, error) {
	var err error
	out := make([]*hash.Hash, len(hashes))
	for i := range hashes {
		out[i], err = NewHash(hashes[i])
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// FromHashSet converts a hash set to a list of plumbing.Hash.
func FromHashSet(hashSet []*hash.Hash) ([]plumbing.Hash, error) {
	var err error
	hashes := make([]plumbing.Hash, len(hashSet))
	for i, h := range hashSet {
		hashes[i], err = FromHash(h)
		if err != nil {
			return nil, err
		}
	}
	return hashes, nil
}

// IsAllZeros checks if the buf is all zeros.
func IsAllZeros(buf []byte) bool {
	for _, b := range buf {
		if b != 0 {
			return false
		}
	}
	return true
}

// ValidateHash checks a hash meant to be converted into a plumbing.Hash
func ValidateHash(h *hash.Hash) error {
	if len(h.GetHash()) == 0 || h.GetHashType() == hash.HashType_HashType_UNKNOWN {
		return ErrEmptyHash
	}
	if len(h.GetHash()) != 20 || h.GetHashType() != GitHashType {
		return ErrHashTypeInvalid
	}
	return nil
}
