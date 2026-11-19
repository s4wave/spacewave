package packahead

import "github.com/s4wave/spacewave/db/block"

// keyTail is the table key of a block.
type keyTail [block.BlockDigestTailLen]byte

// tailOf returns the table key of key, which has at least block.BlockDigestTailLen bytes.
func tailOf(key []byte) keyTail {
	var t keyTail
	copy(t[:], key[len(key)-block.BlockDigestTailLen:])
	return t
}

// location places a value in a local pack.
type location struct {
	// p is the pack holding the value.
	p *pack
	// off is the value's offset in the pack.
	off uint32
	// size is the value's length.
	size uint32
	// read is set by the first Get since the value was written to p.
	read bool
}

// end returns the offset just past the value.
func (l location) end() int64 {
	return int64(l.off) + int64(l.size)
}

// synced reports whether a successful sync covered this location's bytes.
// The caller holds the log lock.
func (l location) synced() bool {
	return l.p.state == packFinal || l.end() <= l.p.synced
}
