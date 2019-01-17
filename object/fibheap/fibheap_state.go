package fibheap

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

var fibRootKey = "fibroot"

// readState reloads the state from the db.
// if the state does not exist, writes it.
func (h *FibbonaciHeap) readState(ctx context.Context) error {
	d, dOk, err := h.db.GetObject(fibRootKey)
	if err != nil {
		return err
	}

	if !dOk {
		return h.writeState()
	}

	return proto.Unmarshal(d, &h.root)
}

// writeState writes state to the db.
func (h *FibbonaciHeap) writeState() error {
	d, err := proto.Marshal(&h.root)
	if err != nil {
		return err
	}

	return h.db.SetObject(fibRootKey, d)
}

// getIDKey returns the key for the given ID.
func (h *FibbonaciHeap) getIDKey(id string) string {
	return id
}

// getEntry gets the entry with the specified ID from the db.
func (h *FibbonaciHeap) getEntry(id string, alloc bool) (*Entry, error) {
	if id == "" {
		return nil, nil
	}

	if entry, ok := h.entryCache[id]; ok {
		return entry, nil
	}

	idKey := h.getIDKey(id)
	d, dOk, err := h.db.GetObject(idKey)
	if err != nil {
		return nil, err
	}

	if !dOk && !alloc {
		return nil, nil
	}

	entry := &Entry{}
	if dOk {
		if err := proto.Unmarshal(d, entry); err != nil {
			return nil, err
		}
	}

	if h.entryCache == nil {
		h.entryCache = make(map[string]*Entry)
	}

	h.entryCache[id] = entry
	return entry, nil
}

// setEntry sets the entry with the specified ID to the db.
func (h *FibbonaciHeap) setEntry(ctx context.Context, id string, entry *Entry) error {
	idKey := h.getIDKey(id)

	dat, err := proto.Marshal(entry)
	if err != nil {
		return err
	}

	return h.db.SetObject(idKey, dat)
}

// editEntry gets an entry, edits it, then writes it back.
func (h *FibbonaciHeap) editEntry(ctx context.Context, id string, cb func(e *Entry) (bool, error)) error {
	_, inCache := h.entryCache[id]
	ent, err := h.getEntry(id, false)
	if err != nil {
		return err
	}

	if ent == nil {
		return errors.Errorf("entry %s not found", id)
	}

	dirty, err := cb(ent)
	if err != nil {
		return err
	}

	if !dirty && !inCache {
		delete(h.entryCache, id)
	}

	return nil
}

// flushEntryCache writes the contents of the entry cache and clears it.
func (h *FibbonaciHeap) flushEntryCache(rerrp *error) (rerr error) {
	defer func() {
		if rerrp != nil && rerr != nil {
			*rerrp = rerr
		}
	}()

	if rerrp != nil && *rerrp != nil {
		// don't save entry changes, due to error
		h.entryCache = nil
	}
	if h.entryCache == nil {
		h.entryCache = make(map[string]*Entry)
		return
	}

	if len(h.entryCache) == 0 {
		return
	}

	// use a temporary sub-context
	tmpCtx := context.Background()
	for k, e := range h.entryCache {
		rerr = h.setEntry(tmpCtx, k, e)
		if rerr != nil {
			break
		}
		delete(h.entryCache, k)
	}

	return nil
}

// getPrevNext returns the previous and next entries for an entry.
func (h *FibbonaciHeap) getPrevNext(
	ent *Entry,
	entKey string,
) (prev *Entry, next *Entry, err error) {
	next, err = h.getEntry(ent.GetNext(), false)
	if err != nil {
		return
	}

	if next == nil {
		err = errors.Errorf("cannot find next: %s -> %s", entKey, ent.GetNext())
		return
	}

	prev, err = h.getEntry(ent.GetPrev(), false)
	if err != nil {
		return
	}

	if prev == nil {
		err = errors.Errorf("cannot find prev: %s -> %s", entKey, ent.GetPrev())
		return
	}

	return
}

// getParentChild returns the parent and child entries for an entry.
func (h *FibbonaciHeap) getParentChild(
	ent *Entry,
	entKey string,
) (parent *Entry, child *Entry, err error) {
	if parentID := ent.GetParent(); parentID != "" {
		parent, err = h.getEntry(parentID, false)
		if err != nil {
			return
		}
	}

	if childID := ent.GetChild(); childID != "" {
		child, err = h.getEntry(childID, false)
		if err != nil {
			return
		}
	}

	return
}
