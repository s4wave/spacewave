package bloom

import (
	"encoding/binary"
	"hash/fnv"
)

// Key is a hashed key. Hash a key once to test it against many filters.
type Key struct {
	// h1 supplies the first bloom bit position.
	h1 uint64
	// h2 supplies the nonzero step between bloom bit positions.
	h2 uint64
}

// NewKey hashes a key for Filter.AddKey and Filter.TestKey.
func NewKey(key []byte) Key {
	// Hash the key into fixed scratch for the two bloom bit hashes.
	h := fnv.New128a()
	_, _ = h.Write(key)
	var scratch [16]byte
	sum := h.Sum(scratch[:0])

	// Split the digest, keeping the step nonzero so each probe advances.
	h1 := binary.BigEndian.Uint64(sum[:8])
	h2 := binary.BigEndian.Uint64(sum[8:])
	if h2 == 0 {
		h2 = 1
	}
	return Key{h1: h1, h2: h2}
}
