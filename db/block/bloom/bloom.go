package bloom

import (
	"hash/fnv"
	"math"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/bitset"
)

// Filter is the runtime bloom filter used by block metadata.
type Filter struct {
	words []uint64
	m     uint
	k     uint
}

// NewFilter creates a bloom filter sized for n entries and p false-positive rate.
func NewFilter(n uint, p float64) *Filter {
	if n == 0 {
		n = 1
	}
	if p <= 0 || p >= 1 {
		p = 0.1
	}
	m := uint(math.Ceil(-float64(n) * math.Log(p) / (math.Ln2 * math.Ln2)))
	if m == 0 {
		m = 1
	}
	k := uint(math.Ceil(float64(m) / float64(n) * math.Ln2))
	if k == 0 {
		k = 1
	}
	return &Filter{
		words: make([]uint64, (m+63)/64),
		m:     m,
		k:     k,
	}
}

// From constructs a runtime bloom filter from serialized words.
func From(words []uint64, m, k uint) *Filter {
	if len(words) == 0 || m == 0 || k == 0 {
		return nil
	}
	cp := make([]uint64, len(words))
	copy(cp, words)
	return &Filter{words: cp, m: m, k: k}
}

// NewBloom constructs a new bloom object from an existing bloom.
// if input is nil, returns nil
func NewBloom(bl *Filter) *BloomFilter {
	if bl == nil {
		return nil
	}
	return &BloomFilter{
		K:      uint32(bl.K()),                                 //nolint:gosec
		M:      uint32(bl.Cap()),                               //nolint:gosec
		BitSet: bitset.NewBitset(bl.Words(), uint32(bl.Cap())), //nolint:gosec
	}
}

// NewBloomBlock constructs a new Bloom block.
func NewBloomBlock() block.Block {
	return &BloomFilter{}
}

// IsNil checks if the object is nil.
func (b *BloomFilter) IsNil() bool {
	return b == nil
}

// IsEmpty checks if the bloom filter is empty.
func (b *BloomFilter) IsEmpty() bool {
	m := b.GetM()
	k := b.GetK()
	return b == nil || k == 0 || m == 0 || len(b.GetBitSet().GetSet()) == 0
}

// Clone clones the bloom filter block.
func (b *BloomFilter) Clone() *BloomFilter {
	if b == nil {
		return nil
	}
	return &BloomFilter{
		K:      b.K,
		M:      b.M,
		BitSet: b.GetBitSet().Clone(),
	}
}

// ToBloomFilter converts the bloom block into a BloomFilter.
// Returns nil if empty.
func (b *BloomFilter) ToBloomFilter() *Filter {
	if b.IsEmpty() {
		return nil
	}

	m := uint(b.GetM())
	k := uint(b.GetK())
	return From(b.GetBitSet().GetSet(), m, k)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *BloomFilter) MarshalBlock() ([]byte, error) {
	return b.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *BloomFilter) UnmarshalBlock(data []byte) error {
	return b.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*BloomFilter)(nil))

// K returns the number of hash functions.
func (f *Filter) K() uint {
	return f.k
}

// Cap returns the number of bits in the filter.
func (f *Filter) Cap() uint {
	return f.m
}

// Words returns the underlying bitset words.
func (f *Filter) Words() []uint64 {
	return f.words
}

// Add inserts a key.
func (f *Filter) Add(key []byte) {
	h1, h2 := bloomHash(key)
	for i := uint(0); i < f.k; i++ {
		f.setBit((h1 + uint64(i)*h2) % uint64(f.m))
	}
}

// AddString inserts a key string.
func (f *Filter) AddString(key string) {
	f.Add([]byte(key))
}

// Test checks if a key may be present.
func (f *Filter) Test(key []byte) bool {
	h1, h2 := bloomHash(key)
	for i := uint(0); i < f.k; i++ {
		if !f.hasBit((h1 + uint64(i)*h2) % uint64(f.m)) {
			return false
		}
	}
	return true
}

// TestString checks if a key string may be present.
func (f *Filter) TestString(key string) bool {
	return f.Test([]byte(key))
}

// Copy returns a deep copy of the filter.
func (f *Filter) Copy() *Filter {
	return From(f.words, f.m, f.k)
}

// Merge ORs another filter into this filter.
func (f *Filter) Merge(other *Filter) error {
	if f == nil || other == nil || f.m != other.m || f.k != other.k || len(f.words) != len(other.words) {
		return ErrIncompatibleBloom
	}
	for i := range f.words {
		f.words[i] |= other.words[i]
	}
	return nil
}

// EstimateFalsePositiveRate estimates the false-positive rate for the filter shape.
func EstimateFalsePositiveRate(m, k, n uint) float64 {
	if m == 0 || k == 0 {
		return 1
	}
	return math.Pow(1-math.Exp(-float64(k)*float64(n)/float64(m)), float64(k))
}

func (f *Filter) setBit(idx uint64) {
	f.words[idx/64] |= uint64(1) << (idx % 64)
}

func (f *Filter) hasBit(idx uint64) bool {
	return f.words[idx/64]&(uint64(1)<<(idx%64)) != 0
}

func bloomHash(key []byte) (uint64, uint64) {
	h := fnv.New128a()
	_, _ = h.Write(key)
	sum := h.Sum(nil)
	h1 := uint64(sum[0])<<56 | uint64(sum[1])<<48 | uint64(sum[2])<<40 | uint64(sum[3])<<32 | uint64(sum[4])<<24 | uint64(sum[5])<<16 | uint64(sum[6])<<8 | uint64(sum[7])
	h2 := uint64(sum[8])<<56 | uint64(sum[9])<<48 | uint64(sum[10])<<40 | uint64(sum[11])<<32 | uint64(sum[12])<<24 | uint64(sum[13])<<16 | uint64(sum[14])<<8 | uint64(sum[15])
	if h2 == 0 {
		h2 = 1
	}
	return h1, h2
}
