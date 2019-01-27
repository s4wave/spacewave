package fibheap

import (
	"context"
	"math"

	"github.com/aperturerobotics/hydra/object"
	"github.com/pkg/errors"
)

// FibbonaciHeap is an implementation of a db backed Fibbonaci heap.
type FibbonaciHeap struct {
	ctx context.Context
	db  object.ObjectStore
}

// NewFibbonaciHeap builds a new Fibbonaci heap, writing state to the db.
func NewFibbonaciHeap(ctx context.Context, db object.ObjectStore) (*FibbonaciHeap, error) {
	return &FibbonaciHeap{
		ctx: ctx,
		db:  db,
	}, nil
}

// Enqueue adds a new key to the heap, re-enqueuing if it already exists.
func (h *FibbonaciHeap) Enqueue(key string, priority float64) (rerr error) {
	tx, err := h.startTx(true)
	if err != nil {
		return err
	}
	defer tx.finish(&rerr)

	entry, err := tx.getEntry(key, false)
	if err != nil {
		return err
	}

	if entry != nil {
		if entry.GetPriority() == priority {
			return nil
		}

		// dequeue
		if err := h.dequeueKeyByID(tx, key, entry); err != nil {
			return err
		}

		entry = nil
	}

	entry = &Entry{
		Next:     key,
		Prev:     key,
		Priority: priority,
	}
	tx.entryCache[key] = entry

	minID := tx.root.Min
	var min *Entry
	if minID != "" {
		min, rerr = tx.getEntry(minID, false)
		if rerr != nil {
			return
		}
	}

	nmink, nmine, err := h.mergeLists(tx, min, entry, minID, key)
	if err != nil {
		return err
	}

	tx.root.Min = nmink
	tx.root.MinPriority = nmine.GetPriority()
	tx.root.Size++
	return nil
}

// IsEmpty checks if the heap is empty.
func (h *FibbonaciHeap) IsEmpty() (bool, error) {
	tx, err := h.startTx(false)
	if err != nil {
		return false, err
	}
	defer tx.finish(nil)
	return tx.root.Min == "", nil
}

// Size returns the number of elements in the heap.
func (h *FibbonaciHeap) Size() (int, error) {
	tx, err := h.startTx(false)
	if err != nil {
		return 0, err
	}
	defer tx.finish(nil)
	return int(tx.root.Size), nil
}

// Min returns the minimum element and priority in the heap.
func (h *FibbonaciHeap) Min() (string, float64, error) {
	tx, err := h.startTx(false)
	if err != nil {
		return "", 0, err
	}
	defer tx.finish(nil)

	return tx.root.Min, tx.root.MinPriority, nil
}

// DequeueMin removes and returns the lowest element.
func (h *FibbonaciHeap) DequeueMin() (rmin string, pmin float64, rerr error) {
	tx, err := h.startTx(true)
	if err != nil {
		return "", 0, err
	}
	defer tx.finish(&rerr)
	if tx.root.Min == "" {
		return "", 0, nil
	}

	var rent *Entry
	rent, rmin, rerr = h.dequeueMinEntry(tx)
	pmin = rent.GetPriority()
	return
}

// DecreaseKey decreases the key of the given element and returns an error if it was not found.
func (h *FibbonaciHeap) DecreaseKey(key string, newPriority float64) (rerr error) {
	tx, err := h.startTx(true)
	if err != nil {
		return err
	}
	defer tx.finish(&rerr)

	minID := tx.root.GetMin()
	if minID == "" {
		return errors.Errorf("not found: %s", key)
	}

	entry, err := tx.getEntry(key, false)
	if err != nil {
		return err
	}
	if entry == nil {
		return errors.Errorf("not found: %s", key)
	}

	if newPriority >= entry.GetPriority() {
		return errors.Errorf("priority %v larger than or equal to old: %v", newPriority, entry.GetPriority())
	}

	return h.decreaseEntry(tx, key, entry, newPriority)
}

// Delete deletes an element from the heap.
// No error is returned if not found.
func (h *FibbonaciHeap) Delete(key string) (rerr error) {
	tx, err := h.startTx(true)
	if err != nil {
		return err
	}
	defer tx.finish(&rerr)

	entry, err := tx.getEntry(key, false)
	if err != nil {
		return err
	}

	if entry == nil {
		return nil
	}

	return h.dequeueKeyByID(tx, key, entry)
}

// Merge merges b into a, enqueuing any keys that do not exist already.
// As a consequence of the operation, any elements already existing in A are removed from B.
// This can be used as a one-time UNIQ operation.
/*
func (h *FibbonaciHeap) Merge(other *FibbonaciHeap) (rerr error) {
	if h == nil || other == nil {
		return errors.New("merge: one of the maps was nil")
	}

	tx, err := h.startTx(true)
	if err != nil {
		return err
	}
	defer tx.finish(&rerr)

	resultSize := tx.root.Size

	// unfortunately, have to remove any keys in other that exist in h.
	// this is to avoid collisions
	otherKeys, err := other.db.ListKeys("")
	if err != nil {
		return err
	}

	// remove any keys that would collide
	for _, key := range otherKeys {
		id := key
		if id == fibRootKey {
			continue
		}
		otherEntry, err := other.getEntry(id, false)
		if err != nil {
			return err
		}

		if otherEntry == nil {
			return errors.Errorf("cannot find entry: %s", id)
		}

		_, hvOk, err := h.db.GetObject(key)
		if err != nil {
			return err
		}

		if hvOk {
			if err := other.dequeueKeyByID(id, otherEntry); err != nil {
				return err
			}
		} else {
			h.entryCache[id] = otherEntry
			resultSize++
		}
	}

	heapMin, err := tx.getEntry(tx.root.Min, false)
	if err != nil {
		return err
	}

	otherMin, err := tx.getEntry(other.root.Min, false)
	if err != nil {
		return err
	}

	resultMinKey, resultMinEntry, err := h.mergeLists(
		heapMin, otherMin,
		tx.root.Min, other.root.Min,
	)
	if err != nil {
		return err
	}

	tx.root.Min = resultMinKey
	tx.root.Size = resultSize
	tx.root.MinPriority = resultMinEntry.GetPriority()
	return h.writeState()
}
*/

// dequeueMinEntry dequeues the min entry and returns it.
func (h *FibbonaciHeap) dequeueMinEntry(tx *tx) (*Entry, string, error) {
	minID := tx.root.GetMin()
	if tx.root.GetSize() == 0 || minID == "" {
		return nil, "", nil
	}

	min, err := tx.getEntry(minID, false)
	if err != nil {
		return nil, "", err
	}

	if min == nil {
		return nil, "", nil
	}

	if min.GetNext() == minID {
		tx.root.Min = ""
		tx.root.MinPriority = 0
	} else {
		minPrev, err := tx.getEntry(min.GetPrev(), false)
		if err != nil {
			return nil, "", err
		}
		if minPrev != nil {
			minPrev.Next = min.Next
		}

		minNext, err := tx.getEntry(min.GetNext(), false)
		if err != nil {
			return nil, "", err
		}
		if minNext != nil {
			minNext.Prev = min.Prev
		}

		tx.root.Min = min.Next
		tx.root.MinPriority = minNext.GetPriority()
	}

	nmin := min
	nminID := tx.root.Min
	if nminID != minID {
		nmin, err = tx.getEntry(nminID, false)
		if err != nil {
			return nil, "", err
		}
	}

	minChildID := min.GetChild()
	if minChildID != "" {
		var err error
		currID := min.Child
		var curr *Entry
		for ok := true; ok; ok = (currID != min.Child) {
			curr, err = tx.getEntry(currID, false)
			if err != nil {
				return nil, "", err
			}

			curr.Parent = ""
			currID = curr.GetNext()
		}
	}

	minChild, err := tx.getEntry(minChildID, false)
	if err != nil {
		return nil, "", err
	}

	nmink, nmine, err := h.mergeLists(tx, nmin, minChild, nminID, minChildID)
	if err != nil {
		return nil, "", err
	}

	tx.root.Size--
	tx.root.Min = nmink
	tx.root.MinPriority = nmine.GetPriority() // includes nil check
	if err := tx.writeState(); err != nil {
		return nil, "", err
	}

	delete(tx.entryCache, minID)
	minIDKey := tx.getIDKey(minID)
	if err := tx.tx.Delete(minIDKey); err != nil {
		return nil, "", err
	}

	if nmine == nil {
		return min, minID, nil
	}

	treeSlice := make([]*Entry, 0, tx.root.Size)
	treeSliceKeys := make([]string, 0, tx.root.Size)
	toVisit := make([]*Entry, 0, tx.root.Size)
	toVisitKeys := make([]string, 0, tx.root.Size)

	// Iterate over root node
	currKey := nmink
	curr := nmine
	for {
		toVisit = append(toVisit, curr)
		toVisitKeys = append(toVisitKeys, currKey)

		currKey = curr.GetNext()
		if currKey == toVisitKeys[0] {
			break
		}

		curr, err = tx.getEntry(currKey, false)
		if err != nil {
			return nil, "", err
		}
	}

	for tvi, curr := range toVisit {
		currKey := toVisitKeys[tvi]

		for {
			for curr.Degree >= int32(len(treeSlice)) {
				treeSlice = append(treeSlice, nil)
				treeSliceKeys = append(treeSliceKeys, "")
			}

			if treeSlice[curr.Degree] == nil {
				treeSlice[curr.Degree] = curr
				treeSliceKeys[curr.Degree] = currKey
				break
			}

			other := treeSlice[curr.Degree]
			otherKey := treeSliceKeys[curr.Degree]
			treeSlice[curr.Degree] = nil
			treeSliceKeys[curr.Degree] = ""

			// Determine which of two trees has the smaller root
			var minT, maxT *Entry
			var minTKey, maxTKey string
			if other.Priority < curr.Priority {
				minT = other
				minTKey = otherKey
				maxT = curr
				maxTKey = currKey
			} else {
				minT = curr
				minTKey = currKey
				maxT = other
				maxTKey = otherKey
			}

			// Break max out of the root list
			// then merge it into min's child list
			maxTNextID := maxT.GetNext()
			maxTNext, err := tx.getEntry(maxTNextID, false)
			if err != nil {
				return nil, "", err
			}

			maxTNext.Prev = maxT.GetPrev()

			maxTPrevID := maxT.GetPrev()
			maxTPrev, err := tx.getEntry(maxTPrevID, false)
			if err != nil {
				return nil, "", err
			}

			maxTPrev.Next = maxT.GetNext()

			// Make it a singleton so that we can merge it
			maxT.Prev = maxTKey
			maxT.Next = maxTKey

			minTChildID := minT.GetChild()
			minTChild, err := tx.getEntry(minTChildID, false)
			if err != nil {
				return nil, "", err
			}

			minT.Child, _, err = h.mergeLists(tx, minTChild, maxT, minTChildID, maxTKey)
			if err != nil {
				return nil, "", err
			}

			// Reparent max appropriately
			maxT.Parent = minTKey

			// Clear max's mark, since it can now lose another child
			maxT.Marked = false

			// Increase min's degree. It has another child.
			minT.Degree++

			// Continue merging this tree
			curr = minT
			currKey = minTKey
		}

		/* Update the global min based on this node.  Note that we compare
		 * for <= instead of < here.  That's because if we just did a
		 * reparent operation that merged two different trees of equal
		 * priority, we need to make sure that the min pointer points to
		 * the root-level one.
		 */
		if curr.GetPriority() <= tx.root.MinPriority {
			tx.root.Min = currKey
			tx.root.MinPriority = curr.GetPriority()
			if err := tx.writeState(); err != nil {
				return nil, "", err
			}
		}
	}

	return min, minID, nil
}

// dequeueKeyByID dequeues a key by ID.
func (h *FibbonaciHeap) dequeueKeyByID(tx *tx, key string, entry *Entry) error {
	// set the priority to -inf
	if err := h.decreaseEntry(tx, key, entry, -math.MaxFloat64); err != nil {
		return err
	}

	_, _, err := h.dequeueMinEntry(tx)
	return err
}

// mergeLists merges two lists.
func (h *FibbonaciHeap) mergeLists(
	tx *tx,
	el1, el2 *Entry,
	el1k, el2k string,
) (string, *Entry, error) {
	switch {
	case el1 == nil && el2 == nil:
		return "", nil, nil
	case el1 != nil && el2 == nil:
		return el1k, el1, nil
	case el1 == nil && el2 != nil:
		return el2k, el2, nil
	}

	oneNext := el1.GetNext()
	el1.Next = el2.GetNext()

	el1NextID := el1.GetNext()
	el1Next, err := tx.getEntry(el1NextID, false)
	if err != nil {
		return "", nil, err
	}

	el1Next.Prev = el1k

	el2.Next = oneNext
	el2NextID := el2.GetNext()
	el2Next, err := tx.getEntry(el2NextID, false)
	if err != nil {
		return "", nil, err
	}
	el2Next.Prev = el2k

	if el1.Priority < el2.Priority {
		return el1k, el1, nil
	}

	return el2k, el2, nil
}

// cutEntry cuts an entry.
func (h *FibbonaciHeap) cutEntry(tx *tx, key string, entry *Entry) (rerr error) {
	if entry == nil {
		var err error
		entry, err = tx.getEntry(key, false)
		if err != nil || entry == nil {
			return err
		}
	}

	entry.Marked = false

	parent, _, err := tx.getParentChild(entry, key)
	if parent == nil {
		return nil
	}

	prev, next, err := tx.getPrevNext(entry, key)
	if err != nil {
		return err
	}

	// Rewire siblings
	if next != entry {
		next.Prev = entry.GetPrev()
		prev.Next = entry.GetNext()
	}

	// Rewrite pointer if this is the representative child node
	if parent.GetChild() == key {
		if entry.GetNext() != key {
			parent.Child = entry.GetNext()
		} else {
			parent.Child = ""
		}
	}

	parent.Degree--
	entry.Prev = key
	entry.Next = key
	min, err := tx.getEntry(tx.root.Min, false)
	if err != nil {
		return err
	}

	nextMinKey, nextMin, err := h.mergeLists(tx, min, entry, tx.root.Min, key)
	if err != nil {
		return err
	}

	if nextMinKey != tx.root.Min {
		tx.root.Min = nextMinKey
		tx.root.MinPriority = nextMin.GetPriority()
	}

	defer func() { entry.Parent = "" }()
	if parent.Marked {
		return h.cutEntry(tx, entry.GetParent(), parent)
	}

	parent.Marked = true
	return nil
}

// decreaseEntry decreases an entry to a priority.
func (h *FibbonaciHeap) decreaseEntry(
	tx *tx,
	key string,
	entry *Entry,
	priority float64,
) error {
	entry.Priority = priority

	parent, _, err := tx.getParentChild(entry, key)
	if err != nil {
		return err
	}

	if parent != nil && entry.Priority <= parent.GetPriority() {
		if err := h.cutEntry(tx, key, entry); err != nil {
			return err
		}
	}

	if entry.Priority <= tx.root.GetMinPriority() {
		tx.root.Min = key
		tx.root.MinPriority = entry.GetPriority()
	}

	return nil
}
