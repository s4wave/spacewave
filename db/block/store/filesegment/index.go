// Package block_store_filesegment serves blocks from byte ranges of files on
// this Device. A range is a block when encoding its bytes with the Space's
// transform hashes to the block's ref; the store checks that on every read.
package block_store_filesegment

import (
	"sync"

	"github.com/s4wave/spacewave/db/block"
)

// Index maps block refs to the file ranges that held them at index time.
// Prototype: an in-memory map; the product keeps it in the Device's local
// store, keyed by (Space, ref) with a path-to-refs reverse index.
type Index struct {
	mu   sync.RWMutex
	refs map[string][]Location
}

// NewIndex constructs an empty Index.
func NewIndex() *Index {
	return &Index{refs: make(map[string][]Location)}
}

// Add records that loc held ref.
func (x *Index) Add(ref *block.BlockRef, loc Location) {
	k := ref.MarshalString()
	x.mu.Lock()
	x.refs[k] = append(x.refs[k], loc)
	x.mu.Unlock()
}

// Lookup returns the locations recorded for ref.
func (x *Index) Lookup(ref *block.BlockRef) []Location {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.refs[ref.MarshalString()]
}

// Len returns the number of indexed refs.
func (x *Index) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.refs)
}
