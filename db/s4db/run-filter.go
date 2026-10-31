package s4db

import (
	"encoding/binary"

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

// runFilter is a filter over the keys of one filter partition.
type runFilter struct {
	// words holds the bits, filterBlockBits/64 words per block.
	words []uint64
}

// newRunFilter returns an empty filter sized for n keys at filterBitsPerKey.
func newRunFilter(n int) *runFilter {
	blocks := max(1, (n*filterBitsPerKey+filterBlockBits-1)/filterBlockBits)
	return &runFilter{words: make([]uint64, blocks*filterBlockBits/64)}
}

// decodeRunFilter reads an encoded filter.
func decodeRunFilter(b []byte) *runFilter {
	f := &runFilter{words: make([]uint64, len(b)/8)}
	for i := range f.words {
		f.words[i] = binary.LittleEndian.Uint64(b[8*i:])
	}
	return f
}

// keyHash returns the hash the filters use for key.
func keyHash(key []byte) uint64 {
	return xxhash.Sum64(key)
}

// probe returns the first word of the block for hash h and the hash itself,
// from which the bit positions follow.
func (f *runFilter) probe(h uint64) (uint64, uint64) {
	blocks := uint64(len(f.words)) / (filterBlockBits / 64)
	return (h >> 32) % blocks * (filterBlockBits / 64), h
}

// addHash records a key by its hash.
func (f *runFilter) addHash(h uint64) {
	base, h := f.probe(h)
	for range filterProbes {
		bit := h % filterBlockBits
		f.words[base+bit/64] |= 1 << (bit % 64)
		h = h*0x9e3779b97f4a7c15 + 1
	}
}

// has reports whether key may have been added.
func (f *runFilter) has(key []byte) bool {
	base, h := f.probe(keyHash(key))
	for range filterProbes {
		bit := h % filterBlockBits
		if f.words[base+bit/64]&(1<<(bit%64)) == 0 {
			return false
		}
		h = h*0x9e3779b97f4a7c15 + 1
	}
	return true
}

// encode returns the filter's bytes.
func (f *runFilter) encode() []byte {
	b := make([]byte, 0, 8*len(f.words))
	for _, w := range f.words {
		b = binary.LittleEndian.AppendUint64(b, w)
	}
	return b
}
