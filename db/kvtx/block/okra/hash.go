//go:build !goscript

package kvtx_block_okra

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"math"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

var okraHasherPool = sync.Pool{
	New: func() any {
		return sha256.New()
	},
}

func borrowOkraHasher() hash.Hash {
	h := okraHasherPool.Get().(hash.Hash)
	h.Reset()
	return h
}

func releaseOkraHasher(h hash.Hash) {
	h.Reset()
	okraHasherPool.Put(h)
}

func finishOkraHash(h hash.Hash) []byte {
	sum := h.Sum(nil)
	return sum[:HashSize]
}

func okraDigest(parts ...[]byte) ([]byte, error) {
	// Borrow a hasher for the Okra digest and release it after use.
	h := borrowOkraHasher()
	defer releaseOkraHasher(h)

	// Hash each digest part in the supplied order.
	for _, part := range parts {
		if _, err := h.Write(part); err != nil {
			return nil, err
		}
	}
	return finishOkraHash(h), nil
}

func hashKeyValue(key, value []byte) ([]byte, error) {
	// Require key and value lengths that fit the Okra hash framing.
	if uint64(len(key)) > math.MaxUint32 || uint64(len(value)) > math.MaxUint32 {
		return nil, errors.New("okra hash input exceeds uint32 length")
	}

	// Borrow a hasher and encode the length-prefixed key.
	var size [4]byte
	h := borrowOkraHasher()
	defer releaseOkraHasher(h)
	binary.BigEndian.PutUint32(size[:], uint32(len(key))) //nolint:gosec // hashKeyValue checks the key length against the uint32 framing field.
	if _, err := h.Write(size[:]); err != nil {
		return nil, err
	}
	if _, err := h.Write(key); err != nil {
		return nil, err
	}

	// Encode the length-prefixed value after the key.
	binary.BigEndian.PutUint32(size[:], uint32(len(value))) //nolint:gosec // hashKeyValue checks the value length against the uint32 framing field.
	if _, err := h.Write(size[:]); err != nil {
		return nil, err
	}
	if _, err := h.Write(value); err != nil {
		return nil, err
	}
	return finishOkraHash(h), nil
}

func hashLeaf(key []byte, valueRef *block.BlockRef, valueIsBlob bool) ([]byte, error) {
	// Encode the leaf value reference for hashing.
	var refData []byte
	var err error
	if valueRef != nil {
		refData, err = valueRef.MarshalVT()
		if err != nil {
			return nil, err
		}
	}

	// Hash the key with its value reference and blob marker.
	valueData := make([]byte, 1+len(refData))
	if valueIsBlob {
		valueData[0] = 1
	}
	copy(valueData[1:], refData)
	return hashKeyValue(key, valueData)
}

func hashEntryRange(entries []*Entry) ([]byte, error) {
	// Borrow a hasher for the entry range and release it after use.
	h := borrowOkraHasher()
	defer releaseOkraHasher(h)

	// Hash the entry hashes in their page order.
	for _, ent := range entries {
		if _, err := h.Write(ent.GetHash()); err != nil {
			return nil, err
		}
	}
	return finishOkraHash(h), nil
}

func hashPage(page *Page) ([]byte, error) {
	// Borrow a hasher and encode the page level.
	h := borrowOkraHasher()
	defer releaseOkraHasher(h)
	var buf [8]byte
	binary.BigEndian.PutUint32(buf[:4], page.GetLevel())
	if _, err := h.Write(buf[:4]); err != nil {
		return nil, err
	}

	// Encode whether the page begins with the anchor entry.
	if page.GetStartsAtAnchor() {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	if _, err := h.Write(buf[:1]); err != nil {
		return nil, err
	}

	// Encode the length-prefixed lower and upper page bounds.
	for _, part := range [][]byte{page.GetLowerBound(), page.GetUpperBound()} {
		if uint64(len(part)) > math.MaxUint32 {
			return nil, errors.New("okra page bound exceeds uint32 length")
		}
		binary.BigEndian.PutUint32(buf[:4], uint32(len(part))) //nolint:gosec // the preceding MaxUint32 check protects the page-bound framing field.
		if _, err := h.Write(buf[:4]); err != nil {
			return nil, err
		}
		if _, err := h.Write(part); err != nil {
			return nil, err
		}
	}

	// Encode the total number of keys covered by the page.
	binary.BigEndian.PutUint64(buf[:], page.GetSize())
	if _, err := h.Write(buf[:]); err != nil {
		return nil, err
	}

	// Encode each page entry in its stored order.
	for _, ent := range page.GetEntries() {
		// Encode the page entry anchor marker.
		if ent.GetAnchor() {
			buf[0] = 1
		} else {
			buf[0] = 0
		}
		if _, err := h.Write(buf[:1]); err != nil {
			return nil, err
		}

		// Encode the entry key and hash with a bounded key length.
		if uint64(len(ent.GetKey())) > math.MaxUint32 {
			return nil, errors.New("okra entry key exceeds uint32 length")
		}
		binary.BigEndian.PutUint32(buf[:4], uint32(len(ent.GetKey()))) //nolint:gosec // the preceding MaxUint32 check protects the entry-key framing field.
		if _, err := h.Write(buf[:4]); err != nil {
			return nil, err
		}
		if _, err := h.Write(ent.GetKey()); err != nil {
			return nil, err
		}
		if _, err := h.Write(ent.GetHash()); err != nil {
			return nil, err
		}

		// Encode the number of keys covered by the entry.
		binary.BigEndian.PutUint64(buf[:], ent.GetSize())
		if _, err := h.Write(buf[:]); err != nil {
			return nil, err
		}
	}
	return finishOkraHash(h), nil
}

func isBoundary(nodeHash []byte) bool {
	if len(nodeHash) < 4 {
		return false
	}
	limit := (uint64(1) << 32) / FanoutDegree
	return uint64(binary.BigEndian.Uint32(nodeHash[:4])) < limit
}
