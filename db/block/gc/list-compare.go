package block_gc

import (
	"bytes"

	"github.com/s4wave/spacewave/db/block"
)

// ListDiff classifies a stored child list against the refs a block's decoded
// bytes name.
type ListDiff int

const (
	// ListEqual is a list naming exactly the decoded refs.
	ListEqual ListDiff = iota
	// ListEmpty is an empty list where the decoded refs name children, as a
	// walk that typed the block as a leaf writes.
	ListEmpty
	// ListShort is a non-empty list missing a decoded ref and naming nothing
	// else. The missing children lack this block as a holder.
	ListShort
	// ListLong is a list naming every decoded ref and more.
	ListLong
	// ListDiffers is a list both missing a decoded ref and naming another.
	ListDiffers
)

// String returns the report label of d.
func (d ListDiff) String() string {
	switch d {
	case ListEqual:
		return "equal"
	case ListEmpty:
		return "empty"
	case ListShort:
		return "shorter"
	case ListLong:
		return "longer"
	case ListDiffers:
		return "differs both ways"
	}
	return "unknown"
}

// CompareChildren classifies the stored list stored against refs, the refs
// the block's decoded bytes name. It reads nothing and allocates only the two
// encodings it merges.
func CompareChildren(stored, refs []*block.BlockRef) ListDiff {
	// Merge the two sorted, distinct key lists, counting the keys only one
	// names.
	has, want := EncodeChildren(stored), EncodeChildren(refs)
	var onlyStored, onlyRefs int
	for len(has) != 0 || len(want) != 0 {
		h, hok := nextBlockKey(has)
		w, wok := nextBlockKey(want)
		if !hok && !wok {
			break
		}
		c := 0
		switch {
		case !hok:
			c = 1
		case !wok:
			c = -1
		default:
			c = bytes.Compare(h, w)
		}
		if c <= 0 {
			has = has[len(h):]
		}
		if c >= 0 {
			want = want[len(w):]
		}
		if c < 0 {
			onlyStored++
		} else if c > 0 {
			onlyRefs++
		}
	}

	// Name the difference.
	switch {
	case onlyStored == 0 && onlyRefs == 0:
		return ListEqual
	case onlyStored != 0 && onlyRefs != 0:
		return ListDiffers
	case onlyStored != 0:
		return ListLong
	case len(stored) == 0:
		return ListEmpty
	}
	return ListShort
}
