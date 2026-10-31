package s4db

import (
	"github.com/cespare/xxhash/v2"
)

// A run's filter is a blocked Bloom filter: 512-bit blocks, each key setting
// filterProbes bits in one block chosen by its hash, so a lookup touches one
// cache line. The hash is xxhash64 of the key, split into the block index and
// the bit positions, which every process computes alike.

// filterBlockBits is the bits in one filter block.
const filterBlockBits = 512

// filterProbes is the bits a key sets.
const filterProbes = 7

// runFilter is a filter over the keys of one filter partition, held as its
// encoded bytes: filterBlockBits/8 bytes per block, bit i of a block in byte
// i/8 at bit i%8, the layout of little-endian 64-bit words. A lookup probes a
// cached partition in place, without decoding or copying it.
type runFilter struct {
	// b holds the bits.
	b []byte
}

// newRunFilter returns an empty filter sized for n keys at filterBitsPerKey.
func newRunFilter(n int) runFilter {
	blocks := max(1, (n*filterBitsPerKey+filterBlockBits-1)/filterBlockBits)
	return runFilter{b: make([]byte, blocks*filterBlockBits/8)}
}

// decodeRunFilter returns the filter encoded in b, which it reads in place;
// b must not change while the filter is in use.
func decodeRunFilter(b []byte) runFilter {
	return runFilter{b: b}
}

// keyHash returns the hash the filters use for key.
func keyHash(key []byte) uint64 {
	return xxhash.Sum64(key)
}

// probe returns the first byte of the block for hash h and the hash itself,
// from which the bit positions follow.
func (f runFilter) probe(h uint64) (uint64, uint64) {
	blocks := uint64(len(f.b)) / (filterBlockBits / 8)
	return (h >> 32) % blocks * (filterBlockBits / 8), h
}

// addHash records a key by its hash.
func (f runFilter) addHash(h uint64) {
	base, h := f.probe(h)
	for range filterProbes {
		bit := h % filterBlockBits
		f.b[base+bit/8] |= 1 << (bit % 8)
		h = h*0x9e3779b97f4a7c15 + 1
	}
}

// has reports whether key may have been added.
func (f runFilter) has(key []byte) bool {
	base, h := f.probe(keyHash(key))
	for range filterProbes {
		bit := h % filterBlockBits
		if f.b[base+bit/8]&(1<<(bit%8)) == 0 {
			return false
		}
		h = h*0x9e3779b97f4a7c15 + 1
	}
	return true
}

// encode returns the filter's bytes.
func (f runFilter) encode() []byte {
	return f.b
}
