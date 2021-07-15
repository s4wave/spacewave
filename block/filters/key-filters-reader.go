package filters

import (
	"strings"

	"github.com/aperturerobotics/hydra/block/quad"
	bbloom "github.com/bits-and-blooms/bloom/v3"
)

// KeyFiltersReader reads information from a KeyFilters object.
type KeyFiltersReader struct {
	// keyFilters is the key filters object
	keyFilters *KeyFilters
	// keyBloom is the key bloom filter
	keyBloom *bbloom.BloomFilter
}

// NewKeyFiltersBuilder constructs a new KeyFiltersBuilder.
// If bloomCapacity is set to 0, bloom filter is left empty.
func NewKeyFiltersReader(keyFilters *KeyFilters) *KeyFiltersReader {
	keyBloom := keyFilters.GetKeyBloom().ToBloomFilter()
	return &KeyFiltersReader{
		keyFilters: keyFilters,
		keyBloom:   keyBloom,
	}
}

// TestObjectKey checks if the object key might match the KeyFilters.
func (r *KeyFiltersReader) TestObjectKey(key string) bool {
	// check prefix
	prefixFilter := r.keyFilters.GetKeyPrefix()
	if len(prefixFilter) != 0 && !strings.HasPrefix(key, prefixFilter) {
		return false
	}

	// check bloom filter
	if r.keyBloom != nil {
		if !r.keyBloom.TestString(key) {
			return false
		}
	}
	// key might have been added to the filter.
	return true
}

// TestQuad tests if quad might match the KeyFilters.
func (r *KeyFiltersReader) TestQuad(gq *quad.Quad) bool {
	quadPrefix := r.keyFilters.GetQuadPrefix()
	if !quadPrefix.IsEmpty() {
		if !strings.HasPrefix(gq.GetSubject(), quadPrefix.GetSubject()) {
			return false
		}
		if !strings.HasPrefix(gq.GetPredicate(), quadPrefix.GetPredicate()) {
			return false
		}
		if !strings.HasPrefix(gq.GetObj(), quadPrefix.GetObj()) {
			return false
		}
		if !strings.HasPrefix(gq.GetLabel(), quadPrefix.GetLabel()) {
			return false
		}
	}
	// test key bloom
	keyBloom := r.keyBloom
	if keyBloom != nil {
		if subj := gq.GetSubject(); subj != "" {
			if !keyBloom.TestString(subj) {
				return false
			}
		}
		if obj := gq.GetObj(); obj != "" {
			if !keyBloom.TestString(obj) {
				return false
			}
		}
	}
	// key might be in filter
	return true
}
