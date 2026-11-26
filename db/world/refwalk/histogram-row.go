package refwalk

import (
	"math/bits"
	"strconv"
)

const (
	// smallestClassBits is log2 of the bound of the smallest size class.
	smallestClassBits = 8
	// SizeClasses is the number of size classes: one per power of two from
	// 256 B through 1 MiB, and one for larger blocks.
	SizeClasses = 14
)

// SizeClass returns the class of a block of size bytes: class i holds the
// blocks of at most 256<<i bytes that are larger than the class below, and
// the last class every block above 1 MiB.
func SizeClass(size uint64) int {
	class := bits.Len64(max(size, 1)-1) - smallestClassBits
	return min(max(class, 0), SizeClasses-1)
}

// SizeClassName names a size class by its bound, such as "≤2 KiB".
func SizeClassName(class int) string {
	if class == SizeClasses-1 {
		return ">1 MiB"
	}
	bound := 256 << class
	switch {
	case bound >= 1<<20:
		return "≤" + strconv.Itoa(bound>>20) + " MiB"
	case bound >= 1<<10:
		return "≤" + strconv.Itoa(bound>>10) + " KiB"
	}
	return "≤" + strconv.Itoa(bound) + " B"
}

// HistogramRow counts the blocks of one type, or of all types.
type HistogramRow struct {
	// Type is the block type's Go type, such as "*okra.Page", or empty for the
	// total of every type.
	Type string
	// Blocks is the number of distinct blocks.
	Blocks uint64
	// Bytes is their stored size.
	Bytes uint64
	// Leaves is the number of blocks that name no other block: a type that
	// holds no refs, or a decoded block whose refs are empty.
	Leaves uint64
	// Sizes counts the blocks in each size class, as SizeClass assigns them.
	Sizes [SizeClasses]uint64
	// SizeBytes is the stored size of the blocks in each size class.
	SizeBytes [SizeClasses]uint64
}

// add counts one block of size bytes.
func (r *HistogramRow) add(size uint64, leaf bool) {
	// Count it in the row and in its size class.
	class := SizeClass(size)
	r.Blocks++
	r.Bytes += size
	r.Sizes[class]++
	r.SizeBytes[class] += size
	if leaf {
		r.Leaves++
	}
}
