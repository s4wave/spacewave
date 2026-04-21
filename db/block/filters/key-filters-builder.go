package filters

import (
	"github.com/aperturerobotics/util/commonprefix"
	bbloom "github.com/bits-and-blooms/bloom/v3"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/block/quad"
)

// keyFiltersFpRate is the hard-coded 10% false-positive rate.
const keyFiltersFpRate = 0.1

// KeyFiltersBuilder buffers information in memory to build a KeyFilters block.
type KeyFiltersBuilder struct {
	// keyBloom is the key bloom filter
	// may be nil
	keyBloom *bbloom.BloomFilter
	// applied indicates if any ops have been applied yet
	applied bool
	// quadPrefix contains prefixes affected by selected graph quads.
	// if !isGraph, will be empty
	quadPrefix *quad.Quad
	// keyPrefix is the common prefix affected by all included operations.
	// if isGraph, will be empty
	keyPrefix string
}

// NewKeyFiltersBuilder constructs a new KeyFiltersBuilder.
// If bloomCapacity is set to 0, bloom filter is left empty.
func NewKeyFiltersBuilder(bloomCapacity int) *KeyFiltersBuilder {
	kfb := &KeyFiltersBuilder{}
	if bloomCapacity > 0 {
		kfb.keyBloom = bbloom.NewWithEstimates(uint(bloomCapacity), keyFiltersFpRate)
	}
	return kfb
}

// BuildKeyFilters builds the key filters object.
func (b *KeyFiltersBuilder) BuildKeyFilters() *KeyFilters {
	return &KeyFilters{
		KeyPrefix:  b.keyPrefix,
		QuadPrefix: b.quadPrefix.Clone(),
		KeyBloom:   bloom.NewBloom(b.keyBloom),
	}
}

// ApplyObjectKey applies an object key to the builder.
func (b *KeyFiltersBuilder) ApplyObjectKey(key string) {
	if key == "" {
		return
	}

	// find common prefixes
	if !b.applied {
		b.applied = true
		b.keyPrefix = key
	} else if len(b.keyPrefix) != 0 {
		b.keyPrefix = commonprefix.Prefix(b.keyPrefix, key)
	}
	// apply to key bloom
	if b.keyBloom != nil {
		_ = b.keyBloom.AddString(key)
	}
}

// ApplyQuad applies a quad to the builder.
func (b *KeyFiltersBuilder) ApplyQuad(gq *quad.Quad) {
	if gq.IsEmpty() {
		return
	}
	// find common prefixes
	if !b.applied {
		b.applied = true
		b.quadPrefix = gq.Clone()
	} else if !b.quadPrefix.IsEmpty() {
		b.quadPrefix.Subject = commonprefix.Prefix(
			b.quadPrefix.Subject,
			gq.GetSubject(),
		)
		b.quadPrefix.Predicate = commonprefix.Prefix(
			b.quadPrefix.Predicate,
			gq.GetPredicate(),
		)
		b.quadPrefix.Obj = commonprefix.Prefix(
			b.quadPrefix.Obj,
			gq.GetObj(),
		)
		b.quadPrefix.Label = commonprefix.Prefix(
			b.quadPrefix.Label,
			gq.GetLabel(),
		)
	}
	// apply to key bloom
	if b.keyBloom != nil {
		if subj := gq.GetSubject(); subj != "" {
			_ = b.keyBloom.AddString(subj)
		}
		if obj := gq.GetObj(); obj != "" {
			_ = b.keyBloom.AddString(obj)
		}
	}
}
