package kvtx_block_okra

import (
	"bytes"
	"context"
	fmt "fmt"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"golang.org/x/sync/errgroup"
)

// loadPage returns the validated Okra page stored at cursor.
func loadPage(ctx context.Context, cursor *block.Cursor) (*Page, error) {
	page, err := block.UnmarshalBlock[Page](ctx, cursor)
	if err != nil {
		return nil, err
	}
	if page == nil {
		return nil, fmt.Errorf("okra page: %w", block.NotFoundError(cursor.GetRef()))
	}
	return page, nil
}

func (t *Tx) getRootPage(ctx context.Context) (*Page, *block.Cursor, error) {
	pageCursor := t.bcs.FollowRef(rootPageRefID, t.root.GetRootPageRef())
	page, err := loadPage(ctx, pageCursor)
	return page, pageCursor, err
}

func (t *Tx) findEntry(ctx context.Context, key []byte) (*Page, *block.Cursor, int, error) {
	// Load the root page to begin the key lookup.
	page, pageCursor, err := t.getRootPage(ctx)
	if err != nil {
		return nil, nil, 0, err
	}

	// Follow the child pages that cover the requested key.
	for page.GetLevel() != 0 {
		idx := page.searchEntry(key)
		if idx < 0 {
			return nil, nil, 0, nil
		}
		pageCursor = page.FollowChild(pageCursor, idx)
		page, err = loadPage(ctx, pageCursor)
		if err != nil {
			return nil, nil, 0, err
		}
		if !page.containsKey(key) {
			return nil, nil, 0, nil
		}
	}

	// Require an exact non-anchor match in the leaf page.
	idx := page.searchEntry(key)
	if idx < 0 {
		return nil, nil, 0, nil
	}
	ent := page.GetEntries()[idx]
	if ent.GetAnchor() || !bytes.Equal(ent.GetKey(), key) {
		return nil, nil, 0, nil
	}
	return page, pageCursor, idx, nil
}

// batchLookup is one key requested by GetBatch.
type batchLookup struct {
	// key is the requested key.
	key []byte
	// index is the position of key in the caller's request.
	index int
}

// batchLoad is one block the batch walk loads, and once loaded the node the
// next level walks: the value of one matched lookup in a leaf, or the child
// page covering a run of lookups in an inner page. Runs are offsets into the
// sorted lookups, so a level needs no slices of its own.
type batchLoad[P walkPage, C any] struct {
	// page and cursor are the loaded child page and its position.
	page   P
	cursor C
	// parent is the index of the node holding the entry in the level above.
	parent int32
	// idx is the entry's index in the parent's page.
	idx int32
	// lo and hi bound the run of lookups under the entry, [lo, hi).
	lo, hi int32
}

// batchWalk is the state of one findEntriesBatch walk.
type batchWalk[P walkPage, C any] struct {
	r       pageReader[P, C]
	lookups []batchLookup
	values  [][]byte
	found   []bool
	// level holds the loaded nodes being expanded; next the loads below them.
	level, next []batchLoad[P, C]
}

// findEntriesBatch resolves lookups into values and found, indexed by each
// lookup's request position. The lookups are sorted by key, so the keys under
// each child page form one contiguous run and child pages load in key order.
// The walk goes one tree level at a time and loads every block the next level
// needs together, so a remote store answers the whole level in a few round
// trips.
func findEntriesBatch[P walkPage, C any](
	ctx context.Context,
	r pageReader[P, C],
	lookups []batchLookup,
	values [][]byte,
	found []bool,
) error {
	// Load the root page the walk starts from.
	page, cursor, err := r.rootPage(ctx)
	if err != nil {
		return err
	}

	// A level holds at most one load per lookup, so the walk sizes its two
	// buffers once and swaps them on every level: a load refers only to its
	// parent in the level above.
	w := &batchWalk[P, C]{
		r:       r,
		lookups: lookups,
		values:  values,
		found:   found,
		level:   make([]batchLoad[P, C], 1, len(lookups)),
		next:    make([]batchLoad[P, C], 0, len(lookups)),
	}
	w.level[0] = batchLoad[P, C]{page: page, cursor: cursor, lo: 0, hi: int32(len(lookups))} //nolint:gosec // a batch holds far fewer than 2^31 lookups.
	for len(w.level) != 0 {
		// Plan the level's loads in key order, then run them together.
		w.next = w.next[:0]
		for i := range w.level {
			w.appendLoads(int32(i))
		}
		if err := w.runLoads(ctx); err != nil {
			return err
		}

		// Descend into the loaded child pages with the lookups within them.
		w.level, w.next = w.descend(), w.level
	}
	return nil
}

// appendLoads appends the loads the lookups under node parent need: the value
// of each exact non-anchor match in a leaf, or the child page of each run of
// keys sharing a child in an inner page. Keys ordered before the page's first
// entry load nothing.
func (w *batchWalk[P, C]) appendLoads(parent int32) {
	node := &w.level[parent]
	page := node.page
	for lo := node.lo; lo < node.hi; {
		// Match a leaf's lookups one key at a time.
		key := w.lookups[lo].key
		idx := page.searchEntry(key)
		if page.GetLevel() == 0 {
			if idx >= 0 && !page.isAnchor(idx) && bytes.Equal(page.entryKey(idx), key) {
				w.next = append(w.next, batchLoad[P, C]{parent: parent, idx: int32(idx), lo: lo, hi: lo + 1}) //nolint:gosec // a page holds far fewer than 2^31 entries.
			}
			lo++
			continue
		}

		// Take the run of keys under the first remaining key's child page.
		hi := lo + 1
		for hi < node.hi && page.searchEntry(w.lookups[hi].key) == idx {
			hi++
		}
		if idx >= 0 {
			w.next = append(w.next, batchLoad[P, C]{parent: parent, idx: int32(idx), lo: lo, hi: hi}) //nolint:gosec // a page holds far fewer than 2^31 entries.
		}
		lo = hi
	}
}

// batchLoadConcurrency bounds the block loads one batch level runs at once.
const batchLoadConcurrency = 16

// runLoads runs the planned loads with at most batchLoadConcurrency in
// flight. Each load goes through the reader, so a block the decoded block
// cache holds costs no store read. Returns the first failure.
func (w *batchWalk[P, C]) runLoads(ctx context.Context) error {
	// Load a single block on the caller's goroutine.
	if len(w.next) == 1 {
		return w.load(ctx, 0)
	}

	// Run the loads together; each writes only its own load and result. The
	// group's context is its own variable: the closure captures it, and
	// reassigning ctx would move ctx to the heap on the single-load path too.
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(batchLoadConcurrency)
	for i := range w.next {
		eg.Go(func() error { return w.load(egCtx, i) })
	}
	return eg.Wait()
}

// load runs planned load i: a leaf's matched value goes straight to the
// result, and a child page onto the load.
func (w *batchWalk[P, C]) load(ctx context.Context, i int) error {
	// Copy a leaf's matched value into the result.
	l := &w.next[i]
	parent := &w.level[l.parent]
	if parent.page.GetLevel() == 0 {
		value, err := w.r.entryValue(ctx, parent.page, parent.cursor, int(l.idx))
		if err != nil {
			return err
		}
		index := w.lookups[l.lo].index
		w.values[index], w.found[index] = value, true
		return nil
	}

	// Load an inner page's child onto the load.
	var err error
	l.page, l.cursor, err = w.r.childPage(ctx, parent.page, parent.cursor, int(l.idx))
	return err
}

// descend compacts the loaded child pages of next into the next level, each
// narrowed to the lookups within its bounds. The lookups are sorted and a
// page's bounds are one key range, so the kept lookups stay one run. Leaf
// values and pages with no lookups left drop out.
func (w *batchWalk[P, C]) descend() []batchLoad[P, C] {
	out := w.next[:0]
	for _, l := range w.next {
		if w.level[l.parent].page.GetLevel() == 0 {
			continue
		}
		for l.lo < l.hi && !l.page.containsKey(w.lookups[l.lo].key) {
			l.lo++
		}
		for l.lo < l.hi && !l.page.containsKey(w.lookups[l.hi-1].key) {
			l.hi--
		}
		if l.lo < l.hi {
			out = append(out, l)
		}
	}
	return out
}

func (p *Page) containsKey(key []byte) bool {
	if !p.GetStartsAtAnchor() && bytes.Compare(key, p.GetLowerBound()) < 0 {
		return false
	}
	if upper := p.GetUpperBound(); len(upper) != 0 && bytes.Compare(key, upper) >= 0 {
		return false
	}
	return true
}

// searchEntry returns the index of the last entry at or before key, or -1
// when key sorts before every entry. The anchor sorts before every key.
func (p *Page) searchEntry(key []byte) int {
	idx, found := slices.BinarySearchFunc(p.GetEntries(), key, compareEntryKey)
	if found {
		return idx
	}
	return idx - 1
}

// compareEntryKey orders an entry against key, placing the anchor first.
func compareEntryKey(ent *Entry, key []byte) int {
	if ent.GetAnchor() {
		return -1
	}
	return bytes.Compare(ent.GetKey(), key)
}

// entryToValue returns an owned copy of the entry's value.
func (t *Tx) entryToValue(ctx context.Context, page *Page, cursor *block.Cursor, index int) ([]byte, error) {
	// Raw inline values need neither a cursor handle nor a storage round trip.
	if raw, ok := page.rawInlineValue(index); ok {
		return bytes.Clone(raw), ctx.Err()
	}
	valueCursor := page.FollowValue(cursor, index)
	if page.GetEntries()[index].GetValueIsBlob() {
		return blob.FetchToBytes(ctx, valueCursor)
	}
	data, _, err := valueCursor.FetchLeaf(ctx)
	return data, err
}
