package segment

import (
	"encoding/binary"
	"math"

	"github.com/pkg/errors"
)

// BloomFilter is a space-efficient probabilistic set for membership tests.
type BloomFilter struct {
	bits      []byte
	bitCount  uint32
	numHashes uint8
}

// NewBloomFilter creates a bloom filter sized for n keys at the target false-positive rate.
func NewBloomFilter(n int, fpr float64) *BloomFilter {
	if n < 1 {
		n = 1
	}
	if fpr <= 0 || fpr >= 1 {
		fpr = 0.001
	}

	// Optimal bit count: m = -n * ln(fpr) / (ln(2))^2
	m := -float64(n) * math.Log(fpr) / (math.Ln2 * math.Ln2)
	bitCount := max(uint32(math.Ceil(m)), 8)

	// Optimal hash count: k = (m/n) * ln(2)
	k := float64(bitCount) / float64(n) * math.Ln2
	numHashes := max(uint8(math.Ceil(k)), 1)

	byteCount := (bitCount + 7) / 8
	return &BloomFilter{
		bits:      make([]byte, byteCount),
		bitCount:  bitCount,
		numHashes: numHashes,
	}
}

// Add inserts a key into the bloom filter.
func (bf *BloomFilter) Add(key []byte) {
	h1, h2 := bloomHash(key)
	for i := uint8(0); i < bf.numHashes; i++ {
		pos := (h1 + uint32(i)*h2) % bf.bitCount
		bf.bits[pos/8] |= 1 << (pos % 8)
	}
}

// MayContain returns true if key might be in the set, false if definitely not.
func (bf *BloomFilter) MayContain(key []byte) bool {
	h1, h2 := bloomHash(key)
	for i := uint8(0); i < bf.numHashes; i++ {
		pos := (h1 + uint32(i)*h2) % bf.bitCount
		if bf.bits[pos/8]&(1<<(pos%8)) == 0 {
			return false
		}
	}
	return true
}

// Encode serializes the bloom filter.
// Format: [num_hashes: u8] [bit_count: u32] [bits: variable]
func (bf *BloomFilter) Encode() []byte {
	buf := make([]byte, 1+4+len(bf.bits))
	buf[0] = bf.numHashes
	binary.BigEndian.PutUint32(buf[1:5], bf.bitCount)
	copy(buf[5:], bf.bits)
	return buf
}

// DecodeBloom parses a bloom filter from buf.
func DecodeBloom(buf []byte) (*BloomFilter, error) {
	if len(buf) < 5 {
		return nil, errors.New("bloom data too short")
	}
	numHashes := buf[0]
	bitCount := binary.BigEndian.Uint32(buf[1:5])
	byteCount := (bitCount + 7) / 8
	if uint32(len(buf)-5) < byteCount {
		return nil, errors.Errorf("bloom bits truncated: have %d, want %d", len(buf)-5, byteCount)
	}
	bits := make([]byte, byteCount)
	copy(bits, buf[5:5+byteCount])
	return &BloomFilter{
		bits:      bits,
		bitCount:  bitCount,
		numHashes: numHashes,
	}, nil
}

// bloomHash computes two independent 32-bit hashes by splitting a 64-bit
// FNV-1a hash. The Kirsch-Mitzenmacher technique derives k hash functions
// as h(i) = h1 + i*h2 from these two values.
func bloomHash(key []byte) (uint32, uint32) {
	// FNV-1a 64-bit
	var h uint64 = 0xcbf29ce484222325
	for _, b := range key {
		h ^= uint64(b)
		h *= 0x100000001b3
	}
	return uint32(h), uint32(h >> 32)
}
