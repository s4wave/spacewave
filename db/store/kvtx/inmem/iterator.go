package store_kvtx_inmem

import (
	"bytes"
	"context"
	"slices"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
)

// Iterator implements the kvtx.Iterator interface.
type Iterator struct {
	// ctx is the context
	ctx context.Context
	// tx is the transaction
	tx *Tx
	// prefix is the key prefix to filter by
	prefix []byte
	// sort indicates if iteration should be sorted
	sort bool
	// reverse indicates if iteration should be reversed
	reverse bool
	// err is any error that occurred
	err error
	// items contains the filtered items
	items []*valType
	// pos is the current position in items
	pos int
	// released indicates if the iterator was closed
	released atomic.Bool
}

// NewIterator constructs a new iterator.
func NewIterator(
	ctx context.Context,
	tx *Tx,
	prefix []byte,
	sort bool,
	reverse bool,
) *Iterator {
	it := &Iterator{
		ctx:     ctx,
		tx:      tx,
		prefix:  prefix,
		sort:    sort,
		reverse: reverse,
	}

	// Build items list with prefix filtering
	if tx.discarded.Load() {
		it.err = kvtx.ErrDiscarded
		return it
	}

	// Use prefix as pivot if non-empty
	var pivot *valType
	if len(prefix) != 0 {
		pivot = &valType{key: prefix}
	}

	// Add items from base tree
	tx.s.tree.Ascend(pivot, func(item *valType) bool {
		if len(prefix) != 0 && !bytes.HasPrefix(item.key, prefix) {
			return false
		}
		searchItem := &valType{key: item.key}
		if tx.write {
			if _, delExists := tx.deleted.Get(searchItem); delExists {
				return true
			}
			if _, addExists := tx.added.Get(searchItem); addExists {
				return true
			}
		}
		it.items = append(it.items, item)
		return true
	})

	// Add items from added tree
	var sortRequired bool
	if tx.write {
		tx.added.Ascend(pivot, func(item *valType) bool {
			if len(prefix) != 0 && !bytes.HasPrefix(item.key, prefix) {
				return false
			}
			it.items = append(it.items, item)
			if sort && len(it.items) != 0 && bytes.Compare(it.items[len(it.items)-1].key, item.key) > 0 {
				sortRequired = true
			}
			return true
		})
	}

	// Sort items if requested and we added items
	if sortRequired {
		slices.SortFunc(it.items, func(a, b *valType) int {
			return bytes.Compare(a.key, b.key)
		})
	}

	// Start before first/after last item depending on direction
	if reverse {
		it.pos = len(it.items)
	} else {
		it.pos = -1
	}
	return it
}

// isReleased returns if the iterator or transaction was released.
func (it *Iterator) isReleased() bool {
	return it.released.Load() || it.tx.discarded.Load()
}

// Err returns any error that has closed the iterator.
func (it *Iterator) Err() error {
	if it.err != nil {
		return it.err
	}
	if it.isReleased() {
		return kvtx.ErrDiscarded
	}
	if it.ctx.Err() != nil {
		return context.Canceled
	}
	return nil
}

// Valid returns if the iterator points to a valid entry.
func (it *Iterator) Valid() bool {
	return it.Err() == nil && it.pos >= 0 && it.pos < len(it.items)
}

// Key returns the current entry key.
func (it *Iterator) Key() []byte {
	if !it.Valid() {
		return nil
	}
	return it.items[it.pos].key
}

// Value returns the current entry value.
func (it *Iterator) Value() ([]byte, error) {
	if !it.Valid() {
		return nil, nil
	}
	return it.items[it.pos].val, nil
}

// ValueCopy copies the value to the given byte slice and returns it.
func (it *Iterator) ValueCopy(dst []byte) ([]byte, error) {
	val, err := it.Value()
	if err != nil || val == nil {
		return nil, err
	}
	if cap(dst) < len(val) {
		dst = make([]byte, len(val))
	} else {
		dst = dst[:len(val)]
	}
	copy(dst, val)
	return dst, nil
}

// Next advances to the next entry.
func (it *Iterator) Next() bool {
	if it.Err() != nil {
		return false
	}
	if it.reverse {
		it.pos--
	} else {
		it.pos++
	}
	return it.Valid()
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
func (it *Iterator) Seek(k []byte) error {
	if it.Err() != nil {
		return it.err
	}

	if len(k) == 0 {
		if it.reverse {
			it.pos = len(it.items) - 1
		} else {
			it.pos = 0
		}
		return nil
	}

	// Binary search for the key
	left, right := 0, len(it.items)-1
	for left <= right {
		mid := (left + right) / 2
		cmp := bytes.Compare(it.items[mid].key, k)
		if cmp == 0 {
			it.pos = mid
			return nil
		} else if cmp < 0 {
			left = mid + 1
		} else {
			right = mid - 1
		}
	}

	if it.reverse {
		it.pos = right
		if it.pos < 0 {
			it.pos = -1
		}
	} else {
		it.pos = left
		if it.pos >= len(it.items) {
			it.pos = -1
		}
	}
	return nil
}

// Close closes the iterator.
func (it *Iterator) Close() {
	if !it.released.Swap(true) {
		it.items = nil
		it.pos = -1
		it.err = kvtx.ErrDiscarded
	}
}

// _ is a type assertion
var _ kvtx.Iterator = ((*Iterator)(nil))
