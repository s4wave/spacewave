//go:build !js && !wasip1 && !windows

package main

import (
	"math"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile"
)

// want is a wanted block placed in a pack by the index.
type want struct {
	// key is the block key, ref the block ref it names.
	key []byte
	ref *block.BlockRef
	// off and size are the block's bytes in the pack when extent is set:
	// the h row held them. A 4-byte row names the pack alone.
	off    uint64
	size   uint32
	extent bool
}

// packWork is the wanted blocks of one pack.
type packWork struct {
	// id is the pack id.
	id string
	// entry is the pack's entry from the index, nil when it has none.
	entry *packfile.PackfileEntry
	// wants are the wanted blocks in the pack.
	wants []want
	// filter is the entry's bloom filter, parsed on first use; nil when the
	// entry has none usable, which rules out no key.
	filter       *bloom.Filter
	filterParsed bool
}

// mayHold reports whether the pack's bloom filter may hold key. A pack
// without a usable filter, of format version 1, may hold any key.
func (p *packWork) mayHold(key []byte) bool {
	// Parse the filter once.
	if !p.filterParsed {
		p.filterParsed = true
		if p.entry.GetBloomFormatVersion() == packfile.BloomFormatVersionV1 {
			var encoded bloom.BloomFilter
			if err := encoded.UnmarshalVT(p.entry.GetBloomFilter()); err == nil {
				p.filter = encoded.ToBloomFilter()
			}
		}
	}

	// Test the key.
	return p.filter == nil || p.filter.Test(key)
}

// counts returns the wanted blocks with and without an extent.
func (p *packWork) counts() (ext, noExt int) {
	for _, w := range p.wants {
		if w.extent {
			ext++
		} else {
			noExt++
		}
	}
	return ext, noExt
}

// planMode is how a pack's wanted blocks are fetched.
type planMode int

const (
	// planRanges reads each wanted block's extent.
	planRanges planMode = iota
	// planWhole reads the whole pack.
	planWhole
)

func (m planMode) String() string {
	if m == planWhole {
		return "whole"
	}
	return "ranges"
}

// planOptions choose between whole packs and ranges.
type planOptions struct {
	// Ranges allows range reads for a pack whose wanted blocks all have
	// extents in their h rows, when they cost fewer bytes than the whole
	// pack. Off, every pack is read whole.
	Ranges bool
	// Overhead is the bytes one request is counted at beyond its body.
	Overhead uint64
}

// infinite is the cost of a plan that cannot run.
const infinite = math.MaxUint64

// wholeCost returns the bytes of reading the pack whole, or infinite when
// its size is unknown: no entry, or an entry of size 0, which no pack has.
func wholeCost(p *packWork, o planOptions) uint64 {
	if p.entry.GetSizeBytes() == 0 {
		return infinite
	}
	return p.entry.GetSizeBytes() + o.Overhead
}

// rangesCost returns the bytes of reading each wanted extent with its
// overhead, or infinite when ranges are off or a wanted block has no extent.
func rangesCost(p *packWork, o planOptions) uint64 {
	if !o.Ranges {
		return infinite
	}
	var cost uint64
	for _, w := range p.wants {
		if !w.extent {
			return infinite
		}
		cost += uint64(w.size) + o.Overhead
	}
	return cost
}

// choosePlan picks how to read a pack, the cheaper in bytes of the whole pack
// and its ranges, and returns the bytes it is expected to cost. ok is false
// when neither can run: the pack's size is unknown and ranges cannot serve.
func choosePlan(p *packWork, o planOptions) (mode planMode, cost uint64, ok bool) {
	whole, ranges := wholeCost(p, o), rangesCost(p, o)
	switch {
	case whole == infinite && ranges == infinite:
		return planWhole, infinite, false
	case ranges < whole:
		return planRanges, ranges, true
	default:
		return planWhole, whole, true
	}
}
