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

// overlayDegree is the overlay tree's branching degree. Each commit
// publishes the overlay to readers, so the next commit copies every node it
// changes, and a random key copies a whole leaf. Nodes smaller than the
// library default halve those copies, at a slightly deeper search.
const overlayDegree = 8

// overlay holds the changes after a checkpoint, its live entries apart from
// its deletes. An iterator steps over the live entries without walking the
// deletes a churned prefix gathers until the next checkpoint.
type overlay struct {
	// live holds the entries that are not deletes.
	live *btree.BTreeG[overlayItem]
	// dead holds the deletes.
	dead *btree.BTreeG[overlayItem]
}

// newOverlay returns an empty overlay.
func newOverlay() *overlay {
	return &overlay{live: newOverlayTree(), dead: newOverlayTree()}
}

// newOverlayTree returns an empty tree of overlay items.
func newOverlayTree() *btree.BTreeG[overlayItem] {
	return btree.NewBTreeGOptions(overlayLess, btree.Options{NoLocks: true, Degree: overlayDegree})
}

// overlayLess orders overlay items by key.
func overlayLess(a, b overlayItem) bool {
	if a.head != b.head {
		return a.head < b.head
	}
	return bytes.Compare(a.key, b.key) < 0
}

// Set adds it, replacing and returning the entry of its key.
func (o *overlay) Set(it overlayItem) (overlayItem, bool) {
	// Add it to its tree, done when it replaces an entry there.
	to, from := o.live, o.dead
	if it.e.del {
		to, from = o.dead, o.live
	}
	if old, ok := to.Set(it); ok {
		return old, true
	}

	// Remove an entry of the other kind. Probe first: a delete copies the
	// nodes on its path even when it misses.
	if _, ok := from.Get(it); !ok {
		return overlayItem{}, false
	}
	return from.Delete(it)
}

// Delete removes the entry it holds, which Get returned.
func (o *overlay) Delete(it overlayItem) {
	from := o.live
	if it.e.del {
		from = o.dead
	}
	from.Delete(it)
}

// Get returns the entry of the probe's key.
func (o *overlay) Get(probe overlayItem) (overlayItem, bool) {
	if it, ok := o.live.Get(probe); ok {
		return it, true
	}
	return o.dead.Get(probe)
}

// getDead returns whether the overlay deletes the probe's key.
func (o *overlay) getDead(probe overlayItem) bool {
	_, ok := o.dead.Get(probe)
	return ok
}

// Scan calls fn with each entry in key order until it returns false.
func (o *overlay) Scan(fn func(overlayItem) bool) {
	// Scan one tree alone when the other is empty.
	switch {
	case o.dead.Len() == 0:
		o.live.Scan(fn)
		return
	case o.live.Len() == 0:
		o.dead.Scan(fn)
		return
	}

	// Merge the two trees, whose keys are disjoint.
	li, di := o.live.Iter(), o.dead.Iter()
	defer li.Release()
	defer di.Release()
	lok, dok := li.First(), di.First()
	for lok || dok {
		if lok && (!dok || overlayLess(li.Item(), di.Item())) {
			if !fn(li.Item()) {
				return
			}
			lok = li.Next()
			continue
		}
		if !fn(di.Item()) {
			return
		}
		dok = di.Next()
	}
}

// Len returns the number of entries.
func (o *overlay) Len() int {
	return o.live.Len() + o.dead.Len()
}

// Copy returns a copy that shares nodes with o until either changes.
func (o *overlay) Copy() *overlay {
	return &overlay{live: o.live.Copy(), dead: o.dead.Copy()}
}
