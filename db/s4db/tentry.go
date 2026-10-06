package s4db

import (
	"bytes"

	"github.com/tidwall/btree"
)

// tentry is one change buffered in a write transaction.
type tentry struct {
	// key is the key.
	key []byte
	// del deletes the key.
	del bool
	// data is the new value.
	data []byte
}

// newChanges returns an empty change buffer.
func newChanges() *btree.BTreeG[tentry] {
	return btree.NewBTreeGOptions(func(a, b tentry) bool {
		return bytes.Compare(a.key, b.key) < 0
	}, btree.Options{NoLocks: true})
}
