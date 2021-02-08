package fibheap

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/kvtx/hashmap"
	"github.com/pkg/errors"
)

var (
	entryPrefix = []byte("e/")
	fibRootKey  = []byte("r")
)

// tx is a internal fibheap tx holder
type tx struct {
	tx kvtx.Tx
	// entryCache map[[]byte]*Entry
	entryCache hashmap.Hashmap[*Entry]
	write      bool
	root       *Root
}

// startTx starts a transaction.
func (h *FibbonaciHeap) startTx(ctx context.Context, write bool) (*tx, error) {
	ktx, err := h.db.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	tx := &tx{
		tx:    ktx,
		write: write,
		root:  &Root{},

		// entryCache: make(map[string]*Entry),
		entryCache: hashmap.NewHashmap[*Entry](),
	}
	if err := tx.readState(ctx); err != nil {
		return nil, err
	}
	return tx, nil
}

// finish finishes the tx populating rerr if necessary
func (t *tx) finish(ctx context.Context, rerr *error) {
	defer t.tx.Discard()
	if rerr == nil || *rerr != nil || !t.write {
		return
	}

	// Collect all entries to process
	var entries []struct {
		key   []byte
		idKey []byte
		value *Entry
	}
	err := t.entryCache.Iterate(ctx, func(ctx context.Context, key []byte, value *Entry) error {
		entries = append(entries, struct {
			key   []byte
			idKey []byte
			value *Entry
		}{
			key:   key,
			idKey: t.getIDKey(key),
			value: value,
		})
		return nil
	})
	if err != nil {
		*rerr = err
		return
	}

	// Process collected entries
	for _, entry := range entries {
		dat, err := entry.value.MarshalVT()
		if err != nil {
			*rerr = err
			return
		}
		if err := t.tx.Set(ctx, entry.idKey, dat); err != nil {
			*rerr = err
			return
		}
		if err := t.entryCache.Delete(ctx, entry.key); err != nil {
			*rerr = err
			return
		}
	}

	if err := t.writeState(ctx); err != nil {
		*rerr = err
	} else {
		*rerr = t.tx.Commit(ctx)
	}
}

// getEntry gets the entry with the specified ID from the db.
func (t *tx) getEntry(ctx context.Context, key []byte, alloc bool) (*Entry, error) {
	if len(key) == 0 {
		return nil, nil
	}

	entry, ok, err := t.entryCache.Get(ctx, key)
	if ok || err != nil {
		return entry, err
	}

	idKey := t.getIDKey(key)
	d, dOk, err := t.tx.Get(ctx, idKey)
	if err != nil {
		return nil, err
	}

	if !dOk && !alloc {
		return nil, nil
	}

	entry = &Entry{}
	if dOk {
		if err := entry.UnmarshalVT(d); err != nil {
			return nil, err
		}
	}
	_ = t.entryCache.Set(ctx, key, entry)
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
	ctx context.Context,
	ent *Entry,
	entKey []byte,
) (prev *Entry, next *Entry, err error) {
	next, err = t.getEntry(ctx, ent.GetNext(), false)
	if err != nil {
		return
	}

	if next == nil {
		err = errors.Errorf("cannot find next: %s -> %s", entKey, ent.GetNext())
		return
	}

	prev, err = t.getEntry(ctx, ent.GetPrev(), false)
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
	ctx context.Context,
	ent *Entry,
	entKey []byte,
) (parent *Entry, child *Entry, err error) {
	if parentID := ent.GetParent(); len(parentID) != 0 {
		parent, err = t.getEntry(ctx, parentID, false)
		if err != nil {
			return
		}
	}

	if childID := ent.GetChild(); len(childID) != 0 {
		child, err = t.getEntry(ctx, childID, false)
		if err != nil {
			return
		}
	}

	return
}

// readState reloads the state from the db.
// if the state does not exist, writes it.
func (t *tx) readState(ctx context.Context) error {
	d, dOk, err := t.tx.Get(ctx, fibRootKey)
	if err != nil {
		return err
	}

	if !dOk {
		return t.writeState(ctx)
	}

	return t.root.UnmarshalVT(d)
}

// writeState writes state to the db.
func (t *tx) writeState(ctx context.Context) error {
	d, err := t.root.MarshalVT()
	if err != nil {
		return err
	}

	return t.tx.Set(ctx, fibRootKey, d)
}

// getIDKey returns the key for the given ID.
func (t *tx) getIDKey(key []byte) []byte {
	return bytes.Join([][]byte{
		entryPrefix,
		key,
	}, nil)
}
