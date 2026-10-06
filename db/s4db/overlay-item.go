package s4db

import (
	"bytes"

	"github.com/tidwall/btree"
)

// overlayItem is an overlay entry as the overlay tree holds it. The key and
// its head sit in the item, so a search compares within the tree's nodes
// and follows no pointer unless two heads tie.
type overlayItem struct {
	// head is the head of key.
	head uint64
	// key is the entry's key.
	key []byte
	// e is the entry; nil in a search probe.
	e *oentry
}

// newOverlayItem returns the item holding e.
func newOverlayItem(e *oentry) overlayItem {
	return overlayItem{head: head(e.key), key: e.key, e: e}
}

// overlayProbe returns an item that searches for key.
func overlayProbe(key []byte) overlayItem {
	return overlayItem{head: head(key), key: key}
}

// newOverlay returns an empty overlay.
func newOverlay() *btree.BTreeG[overlayItem] {
	return btree.NewBTreeGOptions(func(a, b overlayItem) bool {
		if a.head != b.head {
			return a.head < b.head
		}
		return bytes.Compare(a.key, b.key) < 0
	}, btree.Options{NoLocks: true})
}
