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

	// walk holds the path to the current leaf page.
	walk iteratorWalk
	// key and index are the current entry's key and its index in the leaf.
	key   []byte
	index int

	valid   bool
	started bool
	closed  bool
}

// NewIterator constructs an Okra iterator.
func NewIterator(ctx context.Context, tx *Tx, prefix []byte, sort, reverse bool) *Iterator {
	// Bind the iterator to the transaction and its requested key prefix.
	it := &Iterator{
		ctx:     ctx,
		tx:      tx,
		reverse: reverse,
		prefix:  bytes.Clone(prefix),
	}
	if tx.readRoot != nil {
		it.walk = &leafWalk[*flatPage, block.ReadCursor]{t: tx, r: flatPages{tx}}
	} else {
		it.walk = &cursorWalk{leafWalk: leafWalk[*Page, *block.Cursor]{t: tx, r: cursorPages{tx}}}
	}

	// Record a canceled context before starting the iterator.
	if err := ctx.Err(); err != nil {
		it.err = err
		return it
	}

	// Apply the held cursor writes, so the iterator sees them.
	if err := tx.Flush(ctx); err != nil {
		it.err = err
		return it
	}

	// Bound the iterator with the successor of its key prefix.
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
	return i.key
}

// Value returns the current entry value, or nil if not valid.
func (i *Iterator) Value() ([]byte, error) {
	// Require a valid entry before reading the iterator value.
	if !i.Valid() {
		return nil, i.err
	}

	// Stop reading the iterator value if its context was canceled.
	if err := i.checkContext(); err != nil {
		return nil, err
	}

	// Decode the value at the current iterator entry.
	return i.walk.value(i.ctx, i.index)
}

// ValueCopy copies the value to the given byte slice and returns it.
func (i *Iterator) ValueCopy(buf []byte) ([]byte, error) {
	// Copy raw inline values straight from the page into buf.
	if i.Valid() {
		if raw, ok := i.walk.leaf().rawInlineValue(i.index); ok {
			if err := i.checkContext(); err != nil {
				return nil, err
			}
			return append(buf[:0], raw...), nil
		}
	}

	// Copy other values out of their owned fetch result.
	val, err := i.Value()
	if err != nil {
		return nil, err
	}
	return append(buf[:0], val...), nil
}

// Next advances to the next entry and returns Valid.
func (i *Iterator) Next() bool {
	// Stop iterator traversal if its context was canceled.
	if err := i.checkContext(); err != nil {
		return false
	}

	// Seek the initial entry or preserve the exhausted iterator state.
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

	// Advance the iterator in its configured direction.
	if i.reverse {
		return i.previous()
	}
	return i.next()
}

// Seek moves the iterator to the first key >= the provided key, or <= in reverse mode.
func (i *Iterator) Seek(key []byte) error {
	// Stop iterator seeking if its context was canceled.
	if err := i.checkContext(); err != nil {
		return err
	}

	// Release the previous traversal and reset the iterator position.
	i.walk.release()
	i.valid = false
	i.started = true
	if i.tx.root.GetSize() == 0 {
		return nil
	}

	// Seek in the configured direction.
	if i.reverse {
		return i.seekReverse(key)
	}
	return i.seekForward(key)
}

// Close closes the iterator.
func (i *Iterator) Close() {
	i.walk.release()
	i.err = context.Canceled
	i.valid = false
	i.closed = true
}

// ValueCursor returns a cursor located at the current value.
func (i *Iterator) ValueCursor() *block.Cursor {
	// Require a valid entry before following its value cursor.
	if !i.Valid() {
		return nil
	}

	// Stop following the iterator value if its context was canceled.
	if err := i.checkContext(); err != nil {
		return nil
	}

	// Follow the value reference of the current iterator entry.
	cursor, err := i.walk.valueCursor(i.ctx, i.index)
	if err != nil {
		i.err = err
		return nil
	}
	return cursor
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
	// Clamp the forward seek target to the iterator prefix.
	target := i.prefix
	if len(key) != 0 && bytes.Compare(key, target) > 0 {
		target = key
	}

	// Find the leaf page containing the forward seek target.
	if err := i.walk.seek(i.ctx, target, false); err != nil {
		return err
	}
	return i.firstAtOrAfter(target)
}

func (i *Iterator) seekReverse(key []byte) error {
	// Exclude reverse seek targets below the iterator prefix.
	if len(key) != 0 && bytes.Compare(key, i.prefix) < 0 {
		return nil
	}

	// Seek strictly below the prefix upper bound when it limits the target.
	if i.upper != nil && (len(key) == 0 || bytes.Compare(key, i.upper) >= 0) {
		if err := i.walk.seek(i.ctx, i.upper, true); err != nil {
			return err
		}
		return i.lastBefore(i.upper)
	}

	// Find the leaf page containing the reverse seek target.
	if err := i.walk.seek(i.ctx, key, true); err != nil {
		return err
	}
	return i.lastAtOrBefore(key)
}

// firstAtOrAfter moves to the first in-bounds entry at or after key, from the
// current leaf page forward.
func (i *Iterator) firstAtOrAfter(key []byte) error {
	for {
		// Find a qualifying entry within the current leaf page.
		page := i.walk.leaf()
		for idx := range page.Len() {
			if page.isAnchor(idx) || len(key) != 0 && bytes.Compare(page.entryKey(idx), key) < 0 {
				continue
			}
			i.setCurrent(idx)
			return nil
		}

		// Continue to the next page when this page has no qualifying entry.
		ok, err := i.walk.step(i.ctx, false)
		if err != nil || !ok {
			return err
		}
	}
}

// lastAtOrBefore moves to the last in-bounds entry at or before key, from the
// current leaf page backward.
func (i *Iterator) lastAtOrBefore(key []byte) error {
	for {
		// Find a qualifying entry within the current leaf page.
		page := i.walk.leaf()
		for idx := page.Len() - 1; idx >= 0; idx-- {
			if page.isAnchor(idx) || len(key) != 0 && bytes.Compare(page.entryKey(idx), key) > 0 {
				continue
			}
			i.setCurrent(idx)
			return nil
		}

		// Continue to the previous page when this page has no qualifying entry.
		ok, err := i.walk.step(i.ctx, true)
		if err != nil || !ok {
			return err
		}
	}
}

// lastBefore moves to the last in-bounds entry before key, from the current
// leaf page backward.
func (i *Iterator) lastBefore(key []byte) error {
	for {
		// Find a qualifying entry within the current leaf page.
		page := i.walk.leaf()
		for idx := page.Len() - 1; idx >= 0; idx-- {
			if page.isAnchor(idx) || bytes.Compare(page.entryKey(idx), key) >= 0 {
				continue
			}
			i.setCurrent(idx)
			return nil
		}

		// Continue to the previous page when this page has no qualifying entry.
		ok, err := i.walk.step(i.ctx, true)
		if err != nil || !ok {
			return err
		}
	}
}

// next advances to the following entry, loading the next leaf page after the
// current one is exhausted.
func (i *Iterator) next() bool {
	// Advance within the current iterator page when an entry remains.
	if idx := i.index + 1; idx < i.walk.leaf().Len() {
		return i.setCurrent(idx)
	}
	return i.stepPage(false)
}

// previous moves to the preceding entry, loading the previous leaf page after
// the current one is exhausted.
func (i *Iterator) previous() bool {
	// Advance within the current iterator page when an entry remains.
	if idx := i.index - 1; idx >= 0 {
		return i.setCurrent(idx)
	}
	return i.stepPage(true)
}

// stepPage moves to the adjacent leaf page and positions on its first
// qualifying entry in the iterator's direction.
func (i *Iterator) stepPage(reverse bool) bool {
	// Load the adjacent page after exhausting the current page.
	ok, err := i.walk.step(i.ctx, reverse)
	if err != nil {
		i.err = err
		return false
	}
	if !ok {
		i.valid = false
		return false
	}

	// Position the iterator on a qualifying entry in the adjacent page.
	if reverse {
		err = i.lastAtOrBefore(nil)
	} else {
		err = i.firstAtOrAfter(nil)
	}
	if err != nil {
		i.err = err
	}
	return i.Valid()
}

// setCurrent positions the iterator on entry idx of the current leaf page,
// and reports false, ending the iteration, when it is out of bounds.
func (i *Iterator) setCurrent(idx int) bool {
	// Require a non-anchor entry within the iterator prefix bounds.
	page := i.walk.leaf()
	if page.isAnchor(idx) || !i.inBounds(page.entryKey(idx)) {
		i.valid = false
		return false
	}

	// Retain the selected entry as the iterator position.
	i.key = append(i.key[:0], page.entryKey(idx)...)
	i.index = idx
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

// iteratorWalk is an Iterator's path to its current leaf page, in one page
// kind.
type iteratorWalk interface {
	// seek loads the path to the leaf page containing key. An empty key seeks
	// the first leaf page, or the last when last is set.
	seek(ctx context.Context, key []byte, last bool) error
	// step loads the next leaf page, or the previous when reverse is set, and
	// reports false at the end of the tree.
	step(ctx context.Context, reverse bool) (bool, error)
	// leaf returns the current leaf page.
	leaf() walkPage
	// value returns an owned copy of the value of entry index of the leaf.
	value(ctx context.Context, index int) ([]byte, error)
	// valueCursor returns a cursor at the value of entry index of the leaf.
	valueCursor(ctx context.Context, index int) (*block.Cursor, error)
	// release drops the path.
	release()
}

// leafWalk walks the leaf pages of one page kind. It holds the path to its
// current leaf page and, for each page above the leaf that a step has crossed,
// one batch read of that page's children the scan has yet to reach.
type leafWalk[P walkPage, C any] struct {
	t    *Tx
	r    pageReader[P, C]
	path pagePath[P, C]
	// ahead holds, by depth in path, the read-ahead of the page at that depth:
	// at most one page's children per level, a bound the page size sets.
	ahead []readAhead[P]
	// reverse is the direction ahead was read in.
	reverse bool
}

// readAhead is the batch read of one page's children, from the child a step
// entered to the end of the page in the scan's direction.
type readAhead[P walkPage] struct {
	// page is the page whose children were read.
	page P
	// answers holds the children no read has taken, or nil when the batch read
	// fewer than two.
	answers *block.BatchAnswers
}

// seek loads the path to the leaf page containing key. It reads nothing
// ahead, so a seek that reads one entry costs only the lookup; the first step
// reads the rest of the leaf's parent.
func (w *leafWalk[P, C]) seek(ctx context.Context, key []byte, last bool) error {
	// Descend toward the key, or the first or last leaf without one.
	w.ahead = nil
	var err error
	switch {
	case len(key) != 0:
		w.path, err = findPagePath(ctx, w.r, w.t.root, 0, okraNodeKey{key: key})
	case last:
		w.path, err = lastPagePath(ctx, w.r, w.t.root, 0)
	default:
		w.path, err = findPagePath(ctx, w.r, w.t.root, 0, okraNodeKey{anchor: true})
	}
	return err
}

// step loads the adjacent leaf page. The first step into or across a page
// above the leaf reads that page's remaining children in the scan's direction
// with one batch, which later steps take from, so a cold scan waits one store
// read per parent rather than one per leaf.
func (w *leafWalk[P, C]) step(ctx context.Context, reverse bool) (bool, error) {
	// Drop a read-ahead of the other direction, then step on the held ones.
	if reverse != w.reverse {
		w.ahead, w.reverse = nil, reverse
	}
	for _, a := range w.ahead {
		ctx = a.answers.Context(ctx)
	}

	// Keep the current path when there is no adjacent page.
	var path pagePath[P, C]
	var ok bool
	var err error
	if reverse {
		path, ok, err = previousPagePath(ctx, w.r, w.path, 0, w.enter)
	} else {
		path, ok, err = nextPagePath(ctx, w.r, w.path, 0, w.enter)
	}
	if ok {
		w.path = path
	}
	return ok, err
}

// enter reads ahead the children of page, at depth in the path, from index to
// the end of the page in the scan's direction, unless it holds that page's
// read-ahead already, and drops the read-ahead of the deeper pages the step
// leaves.
func (w *leafWalk[P, C]) enter(ctx context.Context, depth int, page P, cursor C, index int) (context.Context, error) {
	// A page the scan is still inside keeps its read-ahead.
	if depth < len(w.ahead) && any(w.ahead[depth].page) == any(page) {
		return ctx, nil
	}

	// Read the page's next children in the scan's direction.
	var b block.BatchLoad
	lo, hi := index, page.Len()
	if w.reverse {
		lo, hi = 0, index+1
	}
	for i := lo; i < hi; i++ {
		w.r.aheadPage(ctx, &b, w.r.childAt(page, cursor, i))
	}
	answers, err := b.Read(ctx)
	if err != nil {
		return ctx, err
	}

	// Replace the read-ahead at depth and below.
	w.ahead = w.ahead[:min(depth, len(w.ahead))]
	for len(w.ahead) < depth {
		w.ahead = append(w.ahead, readAhead[P]{})
	}
	w.ahead = append(w.ahead, readAhead[P]{page: page, answers: answers})
	return answers.Context(ctx), nil
}

// leaf returns the current leaf page.
func (w *leafWalk[P, C]) leaf() walkPage {
	return w.path.leaf().page
}

// value returns an owned copy of the value of entry index of the leaf.
func (w *leafWalk[P, C]) value(ctx context.Context, index int) ([]byte, error) {
	frame := w.path.leaf()
	return w.r.entryValue(ctx, frame.page, frame.cursor, index)
}

// valueCursor returns a cursor at the value of entry index of the leaf.
func (w *leafWalk[P, C]) valueCursor(ctx context.Context, index int) (*block.Cursor, error) {
	frame := w.path.leaf()
	return w.r.valueCursor(ctx, frame.page, frame.cursor, index)
}

// release drops the path and the read-ahead.
func (w *leafWalk[P, C]) release() {
	w.path, w.ahead = nil, nil
}

// cursorWalk walks the mutable cursor tree. Each seek pins the tree it walks
// until the next seek or release, so mutations replace pages without changing
// the iterator's path.
type cursorWalk struct {
	leafWalk[*Page, *block.Cursor]
	// snapshot is the detached root position pinning the walked tree.
	snapshot *block.Cursor
}

// seek pins the current tree, then loads the path to key's leaf page in it.
func (w *cursorWalk) seek(ctx context.Context, key []byte, last bool) error {
	// Load the root page so the detached position pins it.
	if _, _, err := w.t.getRootPage(ctx); err != nil {
		return err
	}
	w.snapshot = w.t.bcs.Detach(true)
	return w.leafWalk.seek(ctx, key, last)
}

// release drops the path and discards the pinned tree.
func (w *cursorWalk) release() {
	for _, page := range w.snapshot.DiscardDetached() {
		discardPages(page)
	}
	w.snapshot = nil
	w.leafWalk.release()
}

// _ is a type assertion
var _ kvtx.BlockIterator = (*Iterator)(nil)
