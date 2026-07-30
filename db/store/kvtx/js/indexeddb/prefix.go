package store_kvtx_indexeddb

import "bytes"

// prefixBound identifies which key a prefix range bound is taken from.
type prefixBound int

const (
	// prefixBoundNone selects no bound, leaving that side unbounded.
	prefixBoundNone prefixBound = iota
	// prefixBoundPrefix selects the prefix itself.
	prefixBoundPrefix
	// prefixBoundKey selects the key the iterator is resuming from.
	prefixBoundKey
	// prefixBoundUpper selects the prefix upper bound.
	prefixBoundUpper
)

// prefixRange describes the key range a prefix iterator opens its cursor on.
type prefixRange struct {
	// lower is the bound the range starts at, always inclusive.
	lower prefixBound
	// upper is the bound the range ends at.
	upper prefixBound
	// upperOpen indicates the upper bound excludes its own value.
	upperOpen bool
	// done indicates the range cannot contain a key, so no cursor is opened.
	done bool
}

// buildPrefixRange decides the range a prefix iterator opens its cursor on.
//
// key is the position the iterator resumes from, empty on the first run. A
// caller may Seek to any key, so key is not necessarily inside the prefix
// range. A key short of the range in the direction of travel clamps to the
// near bound, while a key past the range leaves nothing to return.
func buildPrefixRange(prefix, upper []byte, hasUpper bool, key []byte, reverse bool) prefixRange {
	// full is the whole prefix range, used on the first run and as the clamp.
	full := prefixRange{lower: prefixBoundPrefix}
	if hasUpper {
		full.upper, full.upperOpen = prefixBoundUpper, true
	}
	if len(key) == 0 {
		return full
	}

	if reverse {
		if bytes.Compare(key, prefix) < 0 {
			return prefixRange{done: true}
		}
		if hasUpper && bytes.Compare(key, upper) >= 0 {
			return full
		}
		// Resume below key, including key itself so it is read again.
		return prefixRange{lower: prefixBoundPrefix, upper: prefixBoundKey}
	}

	if hasUpper && bytes.Compare(key, upper) >= 0 {
		return prefixRange{done: true}
	}
	if bytes.Compare(key, prefix) < 0 {
		return full
	}
	// Resume above key, including key itself so it is read again.
	res := full
	res.lower = prefixBoundKey
	return res
}
