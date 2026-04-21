package blob

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// chunkSet holds a set of chunks.
type chunkSet struct {
	v *[]*Chunk
}

// NewChunkSet builds a new chunk set container.
//
// bcs should be located at the chunk set sub-block.
func NewChunkSet(v *[]*Chunk, bcs *block.Cursor) *sbset.SubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&chunkSet{v: v}, bcs)
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *chunkSet) Get(i int) block.SubBlock {
	chunks := *r.v
	if len(chunks) > i {
		return chunks[i]
	}
	return nil
}

// Len returns the number of elements.
func (r *chunkSet) Len() int {
	return len(*r.v)
}

// Set sets the value at the index.
func (r *chunkSet) Set(i int, ref block.SubBlock) {
	chunks := *r.v
	if i < 0 || i >= len(chunks) {
		return
	}
	chunks[i], _ = ref.(*Chunk)
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *chunkSet) Truncate(nlen int) {
	chunks := *r.v
	olen := len(chunks)
	if nlen < 0 || nlen >= olen {
		return
	}
	for i := nlen; i < olen; i++ {
		chunks[i] = nil
	}
}

// _ is a type assertion
var _ sbset.SubBlockContainer = ((*chunkSet)(nil))
