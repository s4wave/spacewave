package kvtx_block_okra

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

// Tx is an Okra k/v transaction.
type Tx struct {
	write bool
	bcs   *block.Cursor
	root  *Root

	tx         *block.Transaction
	rel        func()
	commitOnce sync.Once

	rootChangedCb func(*block.Cursor)
}

// NewTx constructs a new Okra transaction.
func NewTx(
	ctx context.Context,
	bcs *block.Cursor,
	btx *block.Transaction,
	write bool,
	rootChangedCb func(*block.Cursor),
) (*Tx, error) {
	ctx, task := trace.NewTask(ctx, "hydra/kvtx-block-okra/new-tx")
	defer task.End()

	taskCtx, subtask := trace.NewTask(ctx, "hydra/kvtx-block-okra/new-tx/unmarshal-root")
	root, err := block.UnmarshalBlock[*Root](taskCtx, bcs, NewRootBlock)
	subtask.End()
	if err != nil {
		return nil, err
	}
	if err := validateRootAtCursor(root, bcs); err != nil {
		return nil, err
	}
	return &Tx{
		write:         write,
		bcs:           bcs,
		root:          root,
		tx:            btx,
		rootChangedCb: rootChangedCb,
	}, nil
}

func validateRootAtCursor(root *Root, bcs *block.Cursor) error {
	err := root.Validate()
	if err == nil {
		return nil
	}
	if err != block.ErrEmptyBlockRef || root.GetSize() == 0 || !root.GetRootPageRef().GetEmpty() {
		return err
	}
	refs, refsErr := bcs.GetAllRefs(true)
	if refsErr != nil {
		return refsErr
	}
	if refs[rootPageRefID] != nil {
		return nil
	}
	return err
}

// GetCursor returns the cursor pointing to the root of the tree.
func (t *Tx) GetCursor() *block.Cursor {
	return t.bcs
}

// Commit commits the transaction to storage.
func (t *Tx) Commit(ctx context.Context) (cerr error) {
	t.commitOnce.Do(func() {
		if t.write {
			if t.tx != nil {
				br, _, err := t.tx.Write(ctx, true)
				if err != nil {
					cerr = err
				} else if t.rootChangedCb != nil {
					t.bcs.SetRefAtCursor(br, true)
					t.rootChangedCb(t.bcs)
				}
			} else if btx := t.bcs.GetTransaction(); btx != nil {
				_, _, cerr = btx.WriteAtRoot(ctx, false, t.bcs)
			}
		}
		if t.rel != nil {
			t.rel()
		}
	})
	return
}

// Discard cancels the transaction.
func (t *Tx) Discard() {
	t.commitOnce.Do(func() {
		if t.rel != nil {
			t.rel()
		}
	})
}

// Size returns the number of keys in the tree.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	return t.root.GetSize(), ctx.Err()
}

// Exists returns whether or not a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if t.root.GetSize() == 0 {
		return false, nil
	}
	page, _, _, err := t.findEntry(ctx, key)
	return page != nil, err
}

// Get returns the value of the specified key if it exists.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if t.root.GetSize() == 0 {
		return nil, false, nil
	}
	page, cursor, index, err := t.findEntry(ctx, key)
	if err != nil || page == nil {
		return nil, false, err
	}
	value, err := t.entryToValue(ctx, page, cursor, index)
	return value, true, err
}

// GetBatch returns values for multiple keys.
func (t *Tx) GetBatch(ctx context.Context, keys [][]byte) ([][]byte, []bool, error) {
	values := make([][]byte, len(keys))
	found := make([]bool, len(keys))
	lookups := make([]batchLookup, 0, len(keys))
	for i, key := range keys {
		if len(key) == 0 {
			return nil, nil, kvtx.ErrEmptyKey
		}
		lookups = append(lookups, batchLookup{
			key:   key,
			index: i,
		})
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if t.root.GetSize() == 0 || len(lookups) == 0 {
		return values, found, nil
	}
	page, pageCursor, err := t.getRootPage(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := t.findEntriesBatch(ctx, page, pageCursor, lookups, values, found); err != nil {
		return nil, nil, err
	}
	return values, found, nil
}

// GetCursorAtKey returns the cursor at the specified key, if it exists.
func (t *Tx) GetCursorAtKey(ctx context.Context, key []byte) (*block.Cursor, error) {
	if len(key) == 0 {
		return nil, kvtx.ErrEmptyKey
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if t.root.GetSize() == 0 {
		return nil, nil
	}
	page, cursor, index, err := t.findEntry(ctx, key)
	if err != nil || page == nil {
		return nil, err
	}
	return page.FollowValue(cursor, index), nil
}

// Set sets a key to a value.
func (t *Tx) Set(ctx context.Context, key, val []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	valueRef, err := t.buildBlobValue(ctx, val)
	if err != nil {
		return err
	}
	return t.setEntry(ctx, BuildEntry{
		Key:         key,
		ValueRef:    valueRef,
		ValueIsBlob: true,
	})
}

// SetCursorAtKey sets the key to a reference to the object at bcs.
func (t *Tx) SetCursorAtKey(ctx context.Context, key []byte, bcs *block.Cursor, isBlob bool) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	valueRef, err := t.materializeValueCursor(ctx, bcs)
	if err != nil {
		return err
	}
	return t.setEntry(ctx, BuildEntry{
		Key:         key,
		ValueRef:    valueRef,
		ValueIsBlob: isBlob,
	})
}

// Delete removes a key from the tree.
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.root.GetSize() == 0 {
		return nil
	}
	_, err := t.deleteEntry(ctx, key)
	return err
}

// DeleteCursorAtKey deletes the key and returns the cursor to the value.
func (t *Tx) DeleteCursorAtKey(ctx context.Context, key []byte) (*block.Cursor, error) {
	if len(key) == 0 {
		return nil, kvtx.ErrEmptyKey
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if t.root.GetSize() == 0 {
		return nil, nil
	}
	page, cursor, index, err := t.findEntry(ctx, key)
	if err != nil || page == nil {
		return nil, err
	}
	valueCursor := page.FollowValue(cursor, index)
	_, err = t.deleteEntry(ctx, key)
	if err != nil {
		return nil, err
	}
	return valueCursor, nil
}

// GetAndDelete removes a key from the tree returning a value.
func (t *Tx) GetAndDelete(ctx context.Context, key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if t.root.GetSize() == 0 {
		return nil, false, nil
	}
	value, found, err := t.Get(ctx, key)
	if err != nil || !found {
		return nil, found, err
	}
	if _, err := t.deleteEntry(ctx, key); err != nil {
		return nil, false, err
	}
	return value, true, nil
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, val []byte) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.root.GetSize() == 0 {
		return nil
	}
	iter := t.Iterate(ctx, prefix, true, false)
	defer iter.Close()
	if err := iter.Seek(nil); err != nil {
		return err
	}
	for iter.Valid() {
		value, err := iter.Value()
		if err != nil {
			return err
		}
		if err := cb(iter.Key(), value); err != nil {
			return err
		}
		iter.Next()
	}
	return iter.Err()
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.root.GetSize() == 0 {
		return nil
	}
	iter := t.Iterate(ctx, prefix, true, false)
	defer iter.Close()
	if err := iter.Seek(nil); err != nil {
		return err
	}
	for iter.Valid() {
		if err := cb(iter.Key()); err != nil {
			return err
		}
		iter.Next()
	}
	return iter.Err()
}

// Iterate returns an iterator with a given key prefix.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return NewIterator(ctx, t, prefix, sort, reverse)
}

// BlockIterate returns the block iterator.
func (t *Tx) BlockIterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.BlockIterator {
	return NewIterator(ctx, t, prefix, sort, reverse)
}

// _ is a type assertion
var _ kvtx.BlockTx = ((*Tx)(nil))
