//go:build darwin || linux

package s4db

import (
	"hash/maphash"
	"sync/atomic"
)

// filter is a blocked Bloom filter over the keys in an overlay. It only
// gains keys, so every state descending from one checkpoint shares it: a key
// a later commit added is a false positive for an earlier state, never a
// false negative. A checkpoint starts a new filter.
type filter struct {
	// seed keys the hash.
	seed maphash.Seed
	// words holds the bits in blocks of eight words.
	words []atomic.Uint64
}

// filterBlock is the number of words in a block; a key sets filterBits bits
// of one block, so a lookup touches one cache line.
const (
	filterBlock = 8
	filterBits  = 6
)

// newFilter returns a filter sized for an overlay of about limit bytes, at
// ten bits per entry of 32 bytes or more.
func newFilter(limit int64) *filter {
	blocks := max(1, limit*10/32/(filterBlock*64))
	return &filter{seed: maphash.MakeSeed(), words: make([]atomic.Uint64, blocks*filterBlock)}
}

// probe returns the first word of key's block and the hash that picks its
// bits.
func (f *filter) probe(key []byte) (int, uint64) {
	h := maphash.Bytes(f.seed, key)
	blocks := uint64(len(f.words) / filterBlock)
	return int((h>>32)*blocks>>32) * filterBlock, h
}

// add records key. Concurrent readers may see some of its bits first; they
// read a later commit's key, which no reader of an earlier state looks for.
func (f *filter) add(key []byte) {
	at, h := f.probe(key)
	for range filterBits {
		bit := h & (filterBlock*64 - 1)
		f.words[at+int(bit/64)].Or(1 << (bit % 64))
		h >>= 9
	}
}

// has reports whether key may have been added.
func (f *filter) has(key []byte) bool {
	at, h := f.probe(key)
	for range filterBits {
		bit := h & (filterBlock*64 - 1)
		if f.words[at+int(bit/64)].Load()&(1<<(bit%64)) == 0 {
			return false
		}
		h >>= 9
	}
	return true
}
