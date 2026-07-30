package kvtx_block_okra

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Iterator implements sorted Okra page iteration.
type Iterator struct {
	ctx     context.Context
	tx      *Tx
	err     error
	reverse bool

	prefix []byte
	upper  []byte

	current iteratorEntry
	path    okraPagePath
	valid   bool
	started bool
	closed  bool
}

type iteratorEntry struct {
	key []byte

	page   *Page
	cursor *block.Cursor
	index  int
}

// NewIterator constructs an Okra iterator.
func NewIterator(ctx context.Context, tx *Tx, prefix []byte, sort, reverse bool) *Iterator {
	it := &Iterator{
		ctx:     ctx,
		tx:      tx,
		reverse: reverse,
		prefix:  bytes.Clone(prefix),
	}
	if err := ctx.Err(); err != nil {
		it.err = err
		return it
	}
	if len(prefix) != 0 {
		it.upper, _ = kvtx.PrefixSuccessor(prefix)
	}
	return it
}

// Err returns any error that has closed the iterator.
func (i *Iterator) Err() error {
	return i.err
}

// Valid returns if the iterator points to a valid entry.
func (i *Iterator) Valid() bool {
	return i.err == nil && i.valid
}

// Key returns the current entry key, or nil if not valid.
func (i *Iterator) Key() []byte {
	if !i.Valid() {
		return nil
	}
	return i.current.key
}

// Value returns the current entry value, or nil if not valid.
func (i *Iterator) Value() ([]byte, error) {
	if !i.Valid() {
		return nil, i.err
	}
	if err := i.checkContext(); err != nil {
		return nil, err
	}
	ent := i.current
	return i.tx.entryToValue(i.ctx, ent.page, ent.cursor, ent.index)
}

// ValueCopy copies the value to the given byte slice and returns it.
func (i *Iterator) ValueCopy(buf []byte) ([]byte, error) {
	val, err := i.Value()
	if err != nil {
		return nil, err
	}
	return append(buf[:0], val...), nil
}

// Next advances to the next entry and returns Valid.
func (i *Iterator) Next() bool {
	if err := i.checkContext(); err != nil {
		return false
	}
	if !i.valid {
		if i.started {
			return false
		}
		if err := i.Seek(nil); err != nil {
			i.err = err
			return false
		}
		return i.Valid()
	}
	if i.reverse {
		return i.previous()
	}
	return i.next()
}

// Seek moves the iterator to the first key >= the provided key, or <= in reverse mode.
func (i *Iterator) Seek(key []byte) error {
	if err := i.checkContext(); err != nil {
		return err
	}
	i.valid = false
	i.started = true
	if i.tx.root.GetSize() == 0 {
		return nil
	}
	if i.reverse {
		return i.seekReverse(key)
	}
	return i.seekForward(key)
}

// Close closes the iterator.
func (i *Iterator) Close() {
	i.err = context.Canceled
	i.valid = false
	i.closed = true
}

// ValueCursor returns a cursor located at the current value.
func (i *Iterator) ValueCursor() *block.Cursor {
	if !i.Valid() {
		return nil
	}
	if err := i.checkContext(); err != nil {
		return nil
	}
	ent := i.current
	return ent.page.FollowValue(ent.cursor, ent.index)
}

func (i *Iterator) checkContext() error {
	if i.closed {
		return i.err
	}
	if i.err != nil {
		return i.err
	}
	if err := i.ctx.Err(); err != nil {
		i.err = err
		return err
	}
	return nil
}

func (i *Iterator) seekForward(key []byte) error {
	target := i.prefix
	if len(key) != 0 && bytes.Compare(key, target) > 0 {
		target = key
	}
	path, err := i.seekPath(target, false)
	if err != nil {
		return err
	}
	return i.firstAtOrAfter(path, target)
}

func (i *Iterator) seekReverse(key []byte) error {
	if len(key) != 0 && bytes.Compare(key, i.prefix) < 0 {
		return nil
	}
	if i.upper != nil && (len(key) == 0 || bytes.Compare(key, i.upper) >= 0) {
		path, err := i.seekPath(i.upper, true)
		if err != nil {
			return err
		}
		return i.lastBefore(path, i.upper)
	}
	path, err := i.seekPath(key, true)
	if err != nil {
		return err
	}
	return i.lastAtOrBefore(path, key)
}

func (i *Iterator) seekPath(key []byte, last bool) (okraPagePath, error) {
	if len(key) == 0 {
		if last {
			return i.tx.lastPagePath(i.ctx, 0)
		}
		return i.tx.findPagePath(i.ctx, 0, okraNodeKey{anchor: true})
	}
	return i.tx.findPagePath(i.ctx, 0, okraNodeKey{key: key})
}

func (t *Tx) lastPagePath(ctx context.Context, level uint32) (okraPagePath, error) {
	if t.root.GetSize() == 0 || level > t.root.GetHeight() {
		return nil, ErrUnexpectedRootMetadata
	}
	page, cursor, err := t.getRootPage(ctx)
	if err != nil {
		return nil, err
	}
	path := okraPagePath{{page: page, cursor: cursor, index: -1}}
	for page.GetLevel() > level {
		idx := len(page.GetEntries()) - 1
		childCursor := page.FollowChild(cursor, idx)
		child, err := loadPage(ctx, childCursor)
		if err != nil {
			return nil, err
		}
		path = append(path, okraPageFrame{page: child, cursor: childCursor, index: idx})
		page, cursor = child, childCursor
	}
	if page.GetLevel() != level {
		return nil, ErrUnexpectedPageMetadata
	}
	return path, ctx.Err()
}

func (i *Iterator) firstAtOrAfter(path okraPagePath, key []byte) error {
	for {
		frame := path.leaf()
		for idx, ent := range frame.page.GetEntries() {
			if ent.GetAnchor() || len(key) != 0 && bytes.Compare(ent.GetKey(), key) < 0 {
				continue
			}
			if i.setCurrent(path, idx) {
				return nil
			}
			return nil
		}
		next, ok, err := i.tx.nextPagePath(i.ctx, path, 0)
		if err != nil || !ok {
			return err
		}
		path = next
	}
}

func (i *Iterator) lastAtOrBefore(path okraPagePath, key []byte) error {
	for {
		frame := path.leaf()
		for idx := len(frame.page.GetEntries()) - 1; idx >= 0; idx-- {
			ent := frame.page.GetEntries()[idx]
			if ent.GetAnchor() || len(key) != 0 && bytes.Compare(ent.GetKey(), key) > 0 {
				continue
			}
			if i.setCurrent(path, idx) {
				return nil
			}
			return nil
		}
		prev, ok, err := i.tx.previousPagePath(i.ctx, path, 0)
		if err != nil || !ok {
			return err
		}
		path = prev
	}
}

func (i *Iterator) lastBefore(path okraPagePath, key []byte) error {
	for {
		frame := path.leaf()
		for idx := len(frame.page.GetEntries()) - 1; idx >= 0; idx-- {
			ent := frame.page.GetEntries()[idx]
			if ent.GetAnchor() || bytes.Compare(ent.GetKey(), key) >= 0 {
				continue
			}
			if i.setCurrent(path, idx) {
				return nil
			}
			return nil
		}
		prev, ok, err := i.tx.previousPagePath(i.ctx, path, 0)
		if err != nil || !ok {
			return err
		}
		path = prev
	}
}

func (i *Iterator) next() bool {
	idx := i.current.index + 1
	if idx < len(i.current.page.GetEntries()) {
		return i.setCurrent(i.path, idx)
	}
	next, ok, err := i.tx.nextPagePath(i.ctx, i.path, 0)
	if err != nil {
		i.err = err
		return false
	}
	if !ok {
		i.valid = false
		return false
	}
	if err := i.firstAtOrAfter(next, nil); err != nil {
		i.err = err
	}
	return i.Valid()
}

func (i *Iterator) previous() bool {
	idx := i.current.index - 1
	if idx >= 0 {
		return i.setCurrent(i.path, idx)
	}
	prev, ok, err := i.tx.previousPagePath(i.ctx, i.path, 0)
	if err != nil {
		i.err = err
		return false
	}
	if !ok {
		i.valid = false
		return false
	}
	if err := i.lastAtOrBefore(prev, nil); err != nil {
		i.err = err
	}
	return i.Valid()
}

func (i *Iterator) setCurrent(path okraPagePath, idx int) bool {
	frame := path.leaf()
	ent := frame.page.GetEntries()[idx]
	if ent.GetAnchor() || !i.inBounds(ent.GetKey()) {
		i.valid = false
		return false
	}
	i.current = iteratorEntry{
		key:    append(i.current.key[:0], ent.GetKey()...),
		page:   frame.page,
		cursor: frame.cursor,
		index:  idx,
	}
	i.path = append(i.path[:0], path...)
	i.valid = true
	i.started = true
	return true
}

func (i *Iterator) inBounds(key []byte) bool {
	if len(i.prefix) != 0 && !bytes.HasPrefix(key, i.prefix) {
		return false
	}
	if i.upper != nil && bytes.Compare(key, i.upper) >= 0 {
		return false
	}
	return true
}

// _ is a type assertion
var _ kvtx.BlockIterator = ((*Iterator)(nil))
