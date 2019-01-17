package fibheap

import (
	"context"
	"math"
	"sync"

	"github.com/aperturerobotics/hydra/object"
	"github.com/pkg/errors"
)

// FibbonaciHeap is an implementation of a db backed Fibbonaci heap.
type FibbonaciHeap struct {
	mtx        sync.Mutex
	db         object.ObjectStore
	root       Root
	entryCache map[string]*Entry
}

// NewFibbonaciHeap builds a new Fibbonaci heap, writing state to the cursor.
func NewFibbonaciHeap(db object.ObjectStore) (*FibbonaciHeap, error) {
	h := &FibbonaciHeap{
		db: db,
	}
	if err := h.flushEntryCache(nil); err != nil {
		return nil, err
	}

	return h, nil
}

// Enqueue adds a new key to the heap, re-enqueuing if it already exists.
func (h *FibbonaciHeap) Enqueue(key string, priority float64) (rerr error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	defer h.flushEntryCache(&rerr)

	entry, err := h.getEntry(key, false)
	if err != nil {
		return err
	}

	if entry != nil {
		if entry.GetPriority() == priority {
			return nil
		}

		// dequeue
		if err := h.dequeueKeyByID(key, entry); err != nil {
			return err
		}

		entry = nil
	}

	entry = &Entry{
		Next:     key,
		Prev:     key,
		Priority: priority,
	}
	h.entryCache[key] = entry

	minID := h.root.Min
	var min *Entry
	if minID != "" {
		min, rerr = h.getEntry(minID, false)
		if rerr != nil {
			return
		}
	}

	nmink, nmine, err := h.mergeLists(min, entry, minID, key)
	if err != nil {
		return err
	}

	h.root.Min = nmink
	h.root.MinPriority = nmine.GetPriority()
	h.root.Size++

	return h.writeState()
}

// IsEmpty checks if the heap is empty.
func (h *FibbonaciHeap) IsEmpty() bool {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	return h.root.Min == ""
}

// Size returns the number of elements in the heap.
func (h *FibbonaciHeap) Size() int {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	return int(h.root.Size)
}

// Min returns the minimum element and priority in the heap.
func (h *FibbonaciHeap) Min() (string, float64, error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	return h.root.Min, h.root.MinPriority, nil
}

// DequeueMin removes and returns the lowest element.
func (h *FibbonaciHeap) DequeueMin() (rmin string, pmin float64, rerr error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if h.root.Min == "" {
		return "", 0, nil
	}

	defer h.flushEntryCache(&rerr)

	var rent *Entry
	rent, rmin, rerr = h.dequeueMinEntry()
	pmin = rent.GetPriority()
	return
}

// DecreaseKey decreases the key of the given element and returns an error if it was not found.
func (h *FibbonaciHeap) DecreaseKey(key string, newPriority float64) (rerr error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	var entry *Entry
	if h.root.GetMin() != "" {
		defer h.flushEntryCache(&rerr)

		var err error
		entry, err = h.getEntry(key, false)
		if err != nil {
			return err
		}
	}

	if entry == nil {
		return errors.Errorf("not found: %s", key)
	}

	if newPriority >= entry.GetPriority() {
		return errors.Errorf("priority %v larger than or equal to old: %v", newPriority, entry.GetPriority())
	}

	return h.decreaseEntry(key, entry, newPriority)
}

// Delete deletes an element from the heap.
// No error is returned if not found.
func (h *FibbonaciHeap) Delete(ctx context.Context, key string) (rerr error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	defer h.flushEntryCache(&rerr)

	entry, err := h.getEntry(key, false)
	if err != nil {
		return err
	}

	if entry == nil {
		return nil
	}

	return h.dequeueKeyByID(key, entry)
}

// Merge merges b into a, enqueuing any keys that do not exist already.
// As a consequence of the operation, any elements already existing in A are removed from B.
// This can be used as a one-time UNIQ operation.
func (h *FibbonaciHeap) Merge(other *FibbonaciHeap) (rerr error) {
	if h == nil || other == nil {
		return errors.New("merge: one of the maps was nil")
	}

	h.mtx.Lock()
	defer h.mtx.Unlock()
	defer h.flushEntryCache(&rerr)

	other.mtx.Lock()
	defer other.mtx.Unlock()
	defer func() {
		other.entryCache = nil
	}()

	resultSize := h.root.Size

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

	heapMin, err := h.getEntry(h.root.Min, false)
	if err != nil {
		return err
	}

	otherMin, err := h.getEntry(other.root.Min, false)
	if err != nil {
		return err
	}

	resultMinKey, resultMinEntry, err := h.mergeLists(
		heapMin, otherMin,
		h.root.Min, other.root.Min,
	)
	if err != nil {
		return err
	}

	h.root.Min = resultMinKey
	h.root.Size = resultSize
	h.root.MinPriority = resultMinEntry.GetPriority()
	return h.writeState()
}

// dequeueMinEntry dequeues the min entry and returns it.
func (h *FibbonaciHeap) dequeueMinEntry() (*Entry, string, error) {
	minID := h.root.GetMin()
	if h.root.GetSize() == 0 || minID == "" {
		return nil, "", nil
	}

	min, err := h.getEntry(minID, false)
	if err != nil {
		return nil, "", err
	}

	if min == nil {
		return nil, "", nil
	}

	if min.GetNext() == minID {
		h.root.Min = ""
		h.root.MinPriority = 0
	} else {
		minPrev, err := h.getEntry(min.GetPrev(), false)
		if err != nil {
			return nil, "", err
		}
		if minPrev != nil {
			minPrev.Next = min.Next
		}

		minNext, err := h.getEntry(min.GetNext(), false)
		if err != nil {
			return nil, "", err
		}
		if minNext != nil {
			minNext.Prev = min.Prev
		}

		h.root.Min = min.Next
		h.root.MinPriority = minNext.GetPriority()
	}

	nmin := min
	nminID := h.root.Min
	if nminID != minID {
		nmin, err = h.getEntry(nminID, false)
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
			curr, err = h.getEntry(currID, false)
			if err != nil {
				return nil, "", err
			}

			curr.Parent = ""
			currID = curr.GetNext()
		}
	}

	minChild, err := h.getEntry(minChildID, false)
	if err != nil {
		return nil, "", err
	}

	nmink, nmine, err := h.mergeLists(nmin, minChild, nminID, minChildID)
	if err != nil {
		return nil, "", err
	}

	h.root.Size--
	h.root.Min = nmink
	h.root.MinPriority = nmine.GetPriority() // includes nil check
	if err := h.writeState(); err != nil {
		return nil, "", err
	}

	delete(h.entryCache, minID)
	minIDKey := h.getIDKey(minID)
	if err := h.db.DeleteObject(minIDKey); err != nil {
		return nil, "", err
	}

	if nmine == nil {
		return min, minID, nil
	}

	treeSlice := make([]*Entry, 0, h.root.Size)
	treeSliceKeys := make([]string, 0, h.root.Size)
	toVisit := make([]*Entry, 0, h.root.Size)
	toVisitKeys := make([]string, 0, h.root.Size)

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

		curr, err = h.getEntry(currKey, false)
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
			maxTNext, err := h.getEntry(maxTNextID, false)
			if err != nil {
				return nil, "", err
			}

			maxTNext.Prev = maxT.GetPrev()

			maxTPrevID := maxT.GetPrev()
			maxTPrev, err := h.getEntry(maxTPrevID, false)
			if err != nil {
				return nil, "", err
			}

			maxTPrev.Next = maxT.GetNext()

			// Make it a singleton so that we can merge it
			maxT.Prev = maxTKey
			maxT.Next = maxTKey

			minTChildID := minT.GetChild()
			minTChild, err := h.getEntry(minTChildID, false)
			if err != nil {
				return nil, "", err
			}

			minT.Child, _, err = h.mergeLists(minTChild, maxT, minTChildID, maxTKey)
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
		if curr.GetPriority() <= h.root.MinPriority {
			h.root.Min = currKey
			h.root.MinPriority = curr.GetPriority()
			if err := h.writeState(); err != nil {
				return nil, "", err
			}
		}
	}

	return min, minID, nil
}

// dequeueKeyByID dequeues a key by ID.
func (h *FibbonaciHeap) dequeueKeyByID(key string, entry *Entry) error {
	// set the priority to -inf
	if err := h.decreaseEntry(key, entry, -math.MaxFloat64); err != nil {
		return err
	}

	_, _, err := h.dequeueMinEntry()
	return err
}

// mergeLists merges two lists.
func (h *FibbonaciHeap) mergeLists(
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
	el1Next, err := h.getEntry(el1NextID, false)
	if err != nil {
		return "", nil, err
	}

	el1Next.Prev = el1k

	el2.Next = oneNext
	el2NextID := el2.GetNext()
	el2Next, err := h.getEntry(el2NextID, false)
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
func (h *FibbonaciHeap) cutEntry(key string, entry *Entry) (rerr error) {
	if entry == nil {
		var err error
		entry, err = h.getEntry(key, false)
		if err != nil || entry == nil {
			return err
		}
	}

	entry.Marked = false

	parent, _, err := h.getParentChild(entry, key)
	if parent == nil {
		return nil
	}

	prev, next, err := h.getPrevNext(entry, key)
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
	min, err := h.getEntry(h.root.Min, false)
	if err != nil {
		return err
	}

	nextMinKey, nextMin, err := h.mergeLists(min, entry, h.root.Min, key)
	if err != nil {
		return err
	}

	if nextMinKey != h.root.Min {
		h.root.Min = nextMinKey
		h.root.MinPriority = nextMin.GetPriority()
		if err := h.writeState(); err != nil {
			return err
		}
	}

	defer func() { entry.Parent = "" }()
	if parent.Marked {
		return h.cutEntry(entry.GetParent(), parent)
	}

	parent.Marked = true
	return nil
}

// decreaseEntry decreases an entry to a priority.
func (h *FibbonaciHeap) decreaseEntry(
	key string,
	entry *Entry,
	priority float64,
) error {
	entry.Priority = priority

	parent, _, err := h.getParentChild(entry, key)
	if err != nil {
		return err
	}

	if parent != nil && entry.Priority <= parent.GetPriority() {
		if err := h.cutEntry(key, entry); err != nil {
			return err
		}
	}

	if entry.Priority <= h.root.GetMinPriority() {
		h.root.Min = key
		h.root.MinPriority = entry.GetPriority()
		return h.writeState()
	}

	return nil
}
