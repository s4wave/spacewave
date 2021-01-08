package fibheap

import (
	"bytes"
	"context"

	"github.com/Workiva/go-datastructures/trie/ctrie"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/util/hashmap"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

var (
	entryPrefix = []byte("entries/")
	fibRootKey  = []byte("fibroot")
)

// tx is a internal fibheap tx holder
type tx struct {
	ctx context.Context
	tx  kvtx.Tx
	// entryCache map[[]byte]*Entry
	entryCache hashmap.Hashmap
	write      bool
	root       *Root
}

// startTx starts a transaction.
func (h *FibbonaciHeap) startTx(write bool) (*tx, error) {
	ktx, err := h.db.NewTransaction(write)
	if err != nil {
		return nil, err
	}
	tx := &tx{
		ctx:   h.ctx,
		tx:    ktx,
		write: write,
		root:  &Root{},

		// entryCache: make(map[string]*Entry),
		entryCache: hashmap.NewHashmap(),
	}
	if err := tx.readState(); err != nil {
		return nil, err
	}
	return tx, nil
}

// finish finishes the tx populating rerr if necessary
func (t *tx) finish(rerr *error) {
	defer t.tx.Discard()
	if rerr == nil || *rerr != nil || !t.write {
		return
	}

	for k, e := range t.entryCache {
		idKey := t.getIDKey(k)
		dat, err := proto.Marshal(e)
		if err != nil {
			*rerr = err
			return
		}
		if err := t.tx.Set(idKey, dat, 0); err != nil {
			*rerr = err
			return
		}
		delete(t.entryCache, k)
	}

	if err := t.writeState(); err != nil {
		*rerr = err
	} else {
		*rerr = t.tx.Commit(t.ctx)
	}
}

// getEntry gets the entry with the specified ID from the db.
func (t *tx) getEntry(key []byte, alloc bool) (*Entry, error) {
	if id == "" {
		return nil, nil
	}

	if entry, ok := t.entryCache[id]; ok {
		return entry, nil
	}

	idKey := t.getIDKey(id)
	d, dOk, err := t.tx.Get(idKey)
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

	if t.entryCache == nil {
		t.entryCache = make(map[string]*Entry)
	}

	t.entryCache[id] = entry
	return entry, nil
}

// setEntry sets the entry with the specified ID
func (t *tx) setEntry(key []byte, entry *Entry) {
	t.entryCache[id] = entry
}

// editEntry gets an entry, edits it, then writes it back.
func (t *tx) editEntry(key []byte, cb func(e *Entry) (bool, error)) error {
	_, inCache := t.entryCache[id]
	ent, err := t.getEntry(id, false)
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
		delete(t.entryCache, id)
	}

	return nil
}

// getPrevNext returns the previous and next entries for an entry.
func (t *tx) getPrevNext(
	ent *Entry,
	entKey string,
) (prev *Entry, next *Entry, err error) {
	next, err = t.getEntry(ent.GetNext(), false)
	if err != nil {
		return
	}

	if next == nil {
		err = errors.Errorf("cannot find next: %s -> %s", entKey, ent.GetNext())
		return
	}

	prev, err = t.getEntry(ent.GetPrev(), false)
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
func (t *tx) getParentChild(
	ent *Entry,
	entKey string,
) (parent *Entry, child *Entry, err error) {
	if parentID := ent.GetParent(); parentID != "" {
		parent, err = t.getEntry(parentID, false)
		if err != nil {
			return
		}
	}

	if childID := ent.GetChild(); childID != "" {
		child, err = t.getEntry(childID, false)
		if err != nil {
			return
		}
	}

	return
}

// readState reloads the state from the db.
// if the state does not exist, writes it.
func (t *tx) readState() error {
	d, dOk, err := t.tx.Get(fibRootKey)
	if err != nil {
		return err
	}

	if !dOk {
		return t.writeState()
	}

	return proto.Unmarshal(d, t.root)
}

// writeState writes state to the db.
func (t *tx) writeState() error {
	d, err := proto.Marshal(t.root)
	if err != nil {
		return err
	}

	return t.tx.Set(fibRootKey, d, 0)
}

// getIDKey returns the key for the given ID.
func (t *tx) getIDKey(key []byte) []byte {
	return bytes.Join([][]byte{
		entryPrefix,
		[]byte(id),
	}, nil)
}
