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

// newFilter returns a filter sized for an overlay of about limit bytes of
// memory, at ten bits per entry.
func newFilter(limit int64) *filter {
	return &filter{seed: maphash.MakeSeed(), words: make([]atomic.Uint64, max(1, limit*10/entryOverhead/64))}
}

// probe returns the word for key and the bits key sets in it.
func (f *filter) probe(key []byte) (*atomic.Uint64, uint64) {
	i, mask := blockProbe(maphash.Bytes(f.seed, key), len(f.words))
	return &f.words[i], mask
}

// blockProbe returns the word of a blocked Bloom filter of n words that a
// key hashing to h sets, and the bits it sets there: the high half of the
// hash picks the word, the low bits the bits.
func blockProbe(h uint64, n int) (int, uint64) {
	i := int((h >> 32) * uint64(n) >> 32) // #nosec G115 -- the product's high half is below n.
	var mask uint64
	for range filterBits {
		mask |= 1 << (h & 63)
		h >>= 6
	}
	return i, mask
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
