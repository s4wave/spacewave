package packahead

// keyTailLen is the bytes of a key the table keeps: the end of its hash
// digest, 128 bits, as the bucket pack index keeps, so two blocks share a
// row only by a collision no one will find.
const keyTailLen = 16

// keyTail is the table key of a block.
type keyTail [keyTailLen]byte

// tailOf returns the table key of key, which has at least keyTailLen bytes.
func tailOf(key []byte) keyTail {
	var t keyTail
	copy(t[:], key[len(key)-keyTailLen:])
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
	// removed is set when Remove freed the value before its pack uploaded:
	// it stays readable until the upload, reports no durable copy, and a
	// put of it again clears the mark.
	removed bool
}

// end returns the offset just past the value.
func (l location) end() int64 {
	return int64(l.off) + int64(l.size)
}
