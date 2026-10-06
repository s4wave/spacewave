package s4db

import (
	"hash/maphash"
	"sync/atomic"
)

// filterBits is the number of bits a key sets in its word.
const filterBits = 6

// filter is a Bloom filter over the keys in an overlay that keeps each
// key's bits in one word, so adding a key is one atomic OR and a lookup one
// load. It only gains keys, so every state descending from one checkpoint
// shares it: a key a later commit added is a false positive for an earlier
// state, never a false negative. A checkpoint starts a new filter.
type filter struct {
	// seed keys the hash.
	seed maphash.Seed
	// words holds the bits.
	words []atomic.Uint64
}

// newFilter returns a filter sized for an overlay of about limit bytes, at
// ten bits per entry of 32 bytes or more.
func newFilter(limit int64) *filter {
	return &filter{seed: maphash.MakeSeed(), words: make([]atomic.Uint64, max(1, limit*10/32/64))}
}

// probe returns the word for key and the bits key sets in it.
func (f *filter) probe(key []byte) (*atomic.Uint64, uint64) {
	// The high half of the hash picks the word, the low bits the bits.
	h := maphash.Bytes(f.seed, key)
	w := &f.words[(h>>32)*uint64(len(f.words))>>32]
	var mask uint64
	for range filterBits {
		mask |= 1 << (h & 63)
		h >>= 6
	}
	return w, mask
}

// add records key. Concurrent readers may see its bits before the commit
// that added it; they read a later commit's key, which no reader of an
// earlier state looks for.
func (f *filter) add(key []byte) {
	w, mask := f.probe(key)
	w.Or(mask)
}

// has reports whether key may have been added.
func (f *filter) has(key []byte) bool {
	w, mask := f.probe(key)
	return w.Load()&mask == mask
}
