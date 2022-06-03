package fibheap

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/util/hashmap"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

var (
	entryPrefix = []byte("e/")
	fibRootKey  = []byte("r")
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

	err := t.entryCache.Iterate(func(key []byte, value interface{}) error {
		idKey := t.getIDKey(key)
		e := value.(*Entry)
		dat, err := proto.Marshal(e)
		if err != nil {
			return err
		}
		if err := t.tx.Set(idKey, dat); err != nil {
			return err
		}
		t.entryCache.Remove(key)
		return nil
	})
	if err != nil {
		*rerr = err
		return
	}

	if err := t.writeState(); err != nil {
		*rerr = err
	} else {
		*rerr = t.tx.Commit(t.ctx)
	}
}

// getEntry gets the entry with the specified ID from the db.
func (t *tx) getEntry(key []byte, alloc bool) (*Entry, error) {
	if len(key) == 0 {
		return nil, nil
	}

	if entry, ok := t.entryCache.Get(key); ok {
		return entry.(*Entry), nil
	}

	idKey := t.getIDKey(key)
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
	t.entryCache.Set(key, entry)
	return entry, nil
}

// setEntry sets the entry with the specified ID
/*
func (t *tx) setEntry(key []byte, entry *Entry) {
	t.entryCache.Set(key, entry)
}
*/

// editEntry gets an entry, edits it, then writes it back.
/*
func (t *tx) editEntry(key []byte, cb func(e *Entry) (bool, error)) error {
	inCache := t.entryCache.Exists(key)
	ent, err := t.getEntry(key, false)
	if err != nil {
		return err
	}

	if ent == nil {
		return errors.Errorf("entry %s not found", key)
	}

	dirty, err := cb(ent)
	if err != nil {
		return err
	}

	if !dirty && !inCache {
		t.entryCache.Remove(key)
	}

	return nil
}
*/

// getPrevNext returns the previous and next entries for an entry.
func (t *tx) getPrevNext(
	ent *Entry,
	entKey []byte,
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
	entKey []byte,
) (parent *Entry, child *Entry, err error) {
	if parentID := ent.GetParent(); len(parentID) != 0 {
		parent, err = t.getEntry(parentID, false)
		if err != nil {
			return
		}
	}

	if childID := ent.GetChild(); len(childID) != 0 {
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

	return t.tx.Set(fibRootKey, d)
}

// getIDKey returns the key for the given ID.
func (t *tx) getIDKey(key []byte) []byte {
	return bytes.Join([][]byte{
		entryPrefix,
		key,
	}, nil)
}
