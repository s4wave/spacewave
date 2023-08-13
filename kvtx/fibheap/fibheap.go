package fibheap

import (
	"bytes"
	"context"
	"math"

	hydra_heap "github.com/aperturerobotics/hydra/heap"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/pkg/errors"
)

// FibbonaciHeap is an implementation of a db backed Fibbonaci heap.
type FibbonaciHeap struct {
	db kvtx.Store
}

// NewFibbonaciHeap builds a new Fibbonaci heap, writing state to the db.
func NewFibbonaciHeap(db kvtx.Store) (*FibbonaciHeap, error) {
	return &FibbonaciHeap{db: db}, nil
}

// Enqueue adds a new key to the heap, re-enqueuing if it already exists.
func (h *FibbonaciHeap) Enqueue(ctx context.Context, key []byte, priority float64) (rerr error) {
	tx, err := h.startTx(ctx, true)
	if err != nil {
		return err
	}
	defer tx.finish(ctx, &rerr)

	entry, err := tx.getEntry(ctx, key, false)
	if err != nil {
		return err
	}

	if entry != nil {
		entryPriority := entry.GetPriority()
		switch {
		case entryPriority == priority:
			return nil
		case entryPriority > priority:
			// decrease key - faster than dequeue + requeue
			return h.decreaseEntry(ctx, tx, key, entry, priority)
		}

		// dequeue & requeue
		if err := h.dequeueKeyByID(ctx, tx, key, entry); err != nil {
			return err
		}
		entry = nil
	}

	entry = &Entry{
		Next:     key,
		Prev:     key,
		Priority: priority,
	}
	if err := tx.entryCache.Set(ctx, key, entry); err != nil {
		return err
	}

	minID := tx.root.Min
	var min *Entry
	if len(minID) != 0 {
		min, rerr = tx.getEntry(ctx, minID, false)
		if rerr != nil {
			return
		}
	}

	nmink, nmine, err := h.mergeLists(ctx, tx, min, entry, minID, key)
	if err != nil {
		return err
	}

	tx.root.Min = nmink
	tx.root.MinPriority = nmine.GetPriority()
	tx.root.Size++
	return nil
}

// Lookup checks priority of the given key.
// Returns 0, false, nil if not found.
func (h *FibbonaciHeap) Lookup(ctx context.Context, key []byte) (pr float64, found bool, rerr error) {
	tx, err := h.startTx(ctx, false)
	if err != nil {
		return 0, false, err
	}
	defer tx.finish(ctx, &rerr)

	entry, err := tx.getEntry(ctx, key, false)
	if err != nil {
		return 0, false, err
	}
	if entry == nil {
		return 0, false, nil
	}
	return entry.GetPriority(), true, nil
}

// IsEmpty checks if the heap is empty.
func (h *FibbonaciHeap) IsEmpty(ctx context.Context) (bool, error) {
	tx, err := h.startTx(ctx, false)
	if err != nil {
		return false, err
	}
	defer tx.finish(ctx, nil)
	return len(tx.root.Min) == 0, nil
}

// Size returns the number of elements in the heap.
func (h *FibbonaciHeap) Size(ctx context.Context) (uint64, error) {
	tx, err := h.startTx(ctx, false)
	if err != nil {
		return 0, err
	}
	defer tx.finish(ctx, nil)
	return uint64(tx.root.GetSize()), nil
}

// Min returns the minimum element and priority in the heap.
func (h *FibbonaciHeap) Min(ctx context.Context) ([]byte, float64, error) {
	tx, err := h.startTx(ctx, false)
	if err != nil {
		return nil, 0, err
	}
	defer tx.finish(ctx, nil)

	return tx.root.Min, tx.root.MinPriority, nil
}

// DequeueMin removes and returns the lowest element.
func (h *FibbonaciHeap) DequeueMin(ctx context.Context) (rmin []byte, pmin float64, rerr error) {
	tx, err := h.startTx(ctx, true)
	if err != nil {
		return nil, 0, err
	}
	defer tx.finish(ctx, &rerr)
	if len(tx.root.Min) == 0 {
		return nil, 0, nil
	}

	var rent *Entry
	rent, rmin, rerr = h.dequeueMinEntry(ctx, tx)
	pmin = rent.GetPriority()
	return
}

// DecreaseKey decreases the key of the given element and returns an error if it was not found.
func (h *FibbonaciHeap) DecreaseKey(ctx context.Context, key []byte, newPriority float64) (rerr error) {
	tx, err := h.startTx(ctx, true)
	if err != nil {
		return err
	}
	defer tx.finish(ctx, &rerr)

	minID := tx.root.GetMin()
	if len(minID) == 0 {
		return errors.Errorf("not found: %s", key)
	}

	entry, err := tx.getEntry(ctx, key, false)
	if err != nil {
		return err
	}
	if entry == nil {
		return errors.Errorf("not found: %s", key)
	}

	if newPriority >= entry.GetPriority() {
		return errors.Errorf("priority %v larger than or equal to old: %v", newPriority, entry.GetPriority())
	}

	return h.decreaseEntry(ctx, tx, key, entry, newPriority)
}

// Flush deletes all elements in the heap.
func (h *FibbonaciHeap) Flush(ctx context.Context) (rerr error) {
	tx, err := h.startTx(ctx, true)
	if err != nil {
		return err
	}
	defer tx.finish(ctx, &rerr)

	if tx.root.GetSize() == 0 {
		return nil
	}

	// fast delete: drop the entire entry store & re-write root
	err = tx.tx.ScanPrefixKeys(ctx, entryPrefix, func(key []byte) error {
		return tx.tx.Delete(ctx, key)
	})
	if err == nil {
		tx.root.Min = nil
		tx.root.MinPriority = 0
		tx.root.Size = 0
	}
	return err
}

// Delete deletes an element from the heap.
// No error is returned if not found.
func (h *FibbonaciHeap) Delete(ctx context.Context, key []byte) (rerr error) {
	tx, err := h.startTx(ctx, true)
	if err != nil {
		return err
	}
	defer tx.finish(ctx, &rerr)

	entry, err := tx.getEntry(ctx, key, false)
	if err != nil {
		return err
	}

	if entry == nil {
		return nil
	}

	return h.dequeueKeyByID(ctx, tx, key, entry)
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
func (h *FibbonaciHeap) dequeueMinEntry(ctx context.Context, tx *tx) (*Entry, []byte, error) {
	minID := tx.root.GetMin()
	if tx.root.GetSize() == 0 || len(minID) == 0 {
		return nil, nil, nil
	}

	min, err := tx.getEntry(ctx, minID, false)
	if err != nil {
		return nil, nil, err
	}

	if min == nil {
		return nil, nil, nil
	}

	if bytes.Equal(min.GetNext(), minID) {
		tx.root.Min = nil
		tx.root.MinPriority = 0
	} else {
		minPrev, err := tx.getEntry(ctx, min.GetPrev(), false)
		if err != nil {
			return nil, nil, err
		}
		if minPrev != nil {
			minPrev.Next = min.Next
		}

		minNext, err := tx.getEntry(ctx, min.GetNext(), false)
		if err != nil {
			return nil, nil, err
		}
		if minNext != nil {
			minNext.Prev = min.Prev
		}

		tx.root.Min = min.Next
		tx.root.MinPriority = minNext.GetPriority()
	}

	nmin := min
	nminID := tx.root.Min
	if !bytes.Equal(nminID, minID) {
		nmin, err = tx.getEntry(ctx, nminID, false)
		if err != nil {
			return nil, nil, err
		}
	}

	minChildID := min.GetChild()
	if len(minChildID) != 0 {
		var err error
		currID := min.Child
		var curr *Entry
		for ok := true; ok; ok = (!bytes.Equal(currID, min.Child)) {
			curr, err = tx.getEntry(ctx, currID, false)
			if err != nil {
				return nil, nil, err
			}

			curr.Parent = nil
			currID = curr.GetNext()
		}
	}

	minChild, err := tx.getEntry(ctx, minChildID, false)
	if err != nil {
		return nil, nil, err
	}

	nmink, nmine, err := h.mergeLists(ctx, tx, nmin, minChild, nminID, minChildID)
	if err != nil {
		return nil, nil, err
	}

	tx.root.Size--
	tx.root.Min = nmink
	tx.root.MinPriority = nmine.GetPriority() // includes nil check
	if err := tx.writeState(ctx); err != nil {
		return nil, nil, err
	}

	_ = tx.entryCache.Delete(ctx, minID)
	minIDKey := tx.getIDKey(minID)
	if err := tx.tx.Delete(ctx, minIDKey); err != nil {
		return nil, nil, err
	}

	if nmine == nil {
		return min, minID, nil
	}

	treeSlice := make([]*Entry, 0, tx.root.Size)
	treeSliceKeys := make([][]byte, 0, tx.root.Size)
	toVisit := make([]*Entry, 0, tx.root.Size)
	toVisitKeys := make([][]byte, 0, tx.root.Size)

	// Iterate over root node
	currKey := nmink
	curr := nmine
	for {
		toVisit = append(toVisit, curr)
		toVisitKeys = append(toVisitKeys, currKey)

		currKey = curr.GetNext()
		if bytes.Equal(currKey, toVisitKeys[0]) {
			break
		}

		curr, err = tx.getEntry(ctx, currKey, false)
		if err != nil {
			return nil, nil, err
		}
	}

	for tvi, curr := range toVisit {
		currKey := toVisitKeys[tvi]

		for {
			// ensure that treeSlice and treeSliceKeys are at least curr.Degree+1 length.
			if deg := int(curr.Degree); len(treeSlice) <= deg {
				if cap(treeSlice) <= deg {
					nts := make([]*Entry, deg+1)
					copy(nts, treeSlice)
					treeSlice = nts
					ntsk := make([][]byte, deg+1)
					copy(ntsk, treeSliceKeys)
					treeSliceKeys = ntsk
				} else {
					treeSlice = treeSlice[:deg+1]
					treeSliceKeys = treeSliceKeys[:deg+1]
				}
			}
			/*
				for curr.Degree >= int32(len(treeSlice)) {
					treeSlice = append(treeSlice, nil)
					treeSliceKeys = append(treeSliceKeys, nil)
				}
			*/

			if treeSlice[curr.Degree] == nil {
				treeSlice[curr.Degree] = curr
				treeSliceKeys[curr.Degree] = currKey
				break
			}

			other := treeSlice[curr.Degree]
			otherKey := treeSliceKeys[curr.Degree]
			treeSlice[curr.Degree] = nil
			treeSliceKeys[curr.Degree] = nil

			// Determine which of two trees has the smaller root
			var minT, maxT *Entry
			var minTKey, maxTKey []byte
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
			maxTNext, err := tx.getEntry(ctx, maxTNextID, false)
			if err != nil {
				return nil, nil, err
			}

			maxTNext.Prev = maxT.GetPrev()

			maxTPrevID := maxT.GetPrev()
			maxTPrev, err := tx.getEntry(ctx, maxTPrevID, false)
			if err != nil {
				return nil, nil, err
			}

			maxTPrev.Next = maxT.GetNext()

			// Make it a singleton so that we can merge it
			maxT.Prev = maxTKey
			maxT.Next = maxTKey

			minTChildID := minT.GetChild()
			minTChild, err := tx.getEntry(ctx, minTChildID, false)
			if err != nil {
				return nil, nil, err
			}

			minT.Child, _, err = h.mergeLists(ctx, tx, minTChild, maxT, minTChildID, maxTKey)
			if err != nil {
				return nil, nil, err
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
			if err := tx.writeState(ctx); err != nil {
				return nil, nil, err
			}
		}
	}

	return min, minID, nil
}

// dequeueKeyByID dequeues a key by ID.
func (h *FibbonaciHeap) dequeueKeyByID(ctx context.Context, tx *tx, key []byte, entry *Entry) error {
	// set the priority to -inf
	if err := h.decreaseEntry(ctx, tx, key, entry, -math.MaxFloat64); err != nil {
		return err
	}

	_, _, err := h.dequeueMinEntry(ctx, tx)
	return err
}

// mergeLists merges two lists.
func (h *FibbonaciHeap) mergeLists(
	ctx context.Context,
	tx *tx,
	el1, el2 *Entry,
	el1k, el2k []byte,
) ([]byte, *Entry, error) {
	switch {
	case el1 == nil && el2 == nil:
		return nil, nil, nil
	case el1 != nil && el2 == nil:
		return el1k, el1, nil
	case el1 == nil && el2 != nil:
		return el2k, el2, nil
	}

	oneNext := el1.GetNext()
	el1.Next = el2.GetNext()

	el1NextID := el1.GetNext()
	el1Next, err := tx.getEntry(ctx, el1NextID, false)
	if err != nil {
		return nil, nil, err
	}

	el1Next.Prev = el1k

	el2.Next = oneNext
	el2NextID := el2.GetNext()
	el2Next, err := tx.getEntry(ctx, el2NextID, false)
	if err != nil {
		return nil, nil, err
	}
	el2Next.Prev = el2k

	if el1.Priority < el2.Priority {
		return el1k, el1, nil
	}

	return el2k, el2, nil
}

// cutEntry cuts an entry.
func (h *FibbonaciHeap) cutEntry(ctx context.Context, tx *tx, key []byte, entry *Entry) (rerr error) {
	if entry == nil {
		var err error
		entry, err = tx.getEntry(ctx, key, false)
		if err != nil || entry == nil {
			return err
		}
	}

	entry.Marked = false

	parent, _, err := tx.getParentChild(ctx, entry, key)
	if err != nil {
		return err
	}
	if parent == nil {
		return nil
	}

	prev, next, err := tx.getPrevNext(ctx, entry, key)
	if err != nil {
		return err
	}

	// Rewire siblings
	if next != entry {
		next.Prev = entry.GetPrev()
		prev.Next = entry.GetNext()
	}

	// Rewrite pointer if this is the representative child node
	if bytes.Equal(parent.GetChild(), key) {
		if !bytes.Equal(entry.GetNext(), key) {
			parent.Child = entry.GetNext()
		} else {
			parent.Child = nil
		}
	}

	parent.Degree--
	entry.Prev = key
	entry.Next = key
	min, err := tx.getEntry(ctx, tx.root.Min, false)
	if err != nil {
		return err
	}

	nextMinKey, nextMin, err := h.mergeLists(ctx, tx, min, entry, tx.root.Min, key)
	if err != nil {
		return err
	}

	if !bytes.Equal(nextMinKey, tx.root.Min) {
		tx.root.Min = nextMinKey
		tx.root.MinPriority = nextMin.GetPriority()
	}

	defer func() { entry.Parent = nil }()
	if parent.Marked {
		return h.cutEntry(ctx, tx, entry.GetParent(), parent)
	}

	parent.Marked = true
	return nil
}

// decreaseEntry decreases an entry to a priority.
func (h *FibbonaciHeap) decreaseEntry(
	ctx context.Context,
	tx *tx,
	key []byte,
	entry *Entry,
	priority float64,
) error {
	entry.Priority = priority

	parent, _, err := tx.getParentChild(ctx, entry, key)
	if err != nil {
		return err
	}

	if parent != nil && entry.Priority <= parent.GetPriority() {
		if err := h.cutEntry(ctx, tx, key, entry); err != nil {
			return err
		}
	}

	if entry.Priority <= tx.root.GetMinPriority() {
		tx.root.Min = key
		tx.root.MinPriority = entry.GetPriority()
	}

	return nil
}

// _ is a type assertion
var _ hydra_heap.Heap = ((*FibbonaciHeap)(nil))
