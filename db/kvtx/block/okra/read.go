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

// batchNode is a page the batch walk reached, with the lookups under it.
type batchNode[P walkPage, C any] struct {
	// page is the loaded page.
	page P
	// cursor locates page.
	cursor C
	// lookups are the requested keys within the page bounds, sorted by key.
	lookups []batchLookup
}

// findEntriesBatch resolves lookups into values and found, indexed by each
// lookup's request position. The lookups are sorted by key, so the keys under
// each child page form one contiguous run and child pages load in key order.
// The walk goes one tree level at a time and loads every block the next level
// needs together, so a remote store answers the whole level in a few round
// trips. findEntriesBatch reorders lookups in place.
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

	// A level holds at most one load and one child node per lookup, so the
	// walk sizes its buffers once and reuses them on every level: loads and
	// the next level refer only to the current level's nodes.
	loads := make([]batchLoad[P, C], 0, len(lookups))
	level := make([]batchNode[P, C], 1, len(lookups))
	next := make([]batchNode[P, C], 0, len(lookups))
	level[0] = batchNode[P, C]{page: page, cursor: cursor, lookups: lookups}
	for len(level) != 0 {
		// Plan the level's loads in key order, then run them together.
		loads = loads[:0]
		for i := range level {
			loads = appendBatchLoads(loads, &level[i])
		}
		if err := runBatchLoads(ctx, r, loads); err != nil {
			return err
		}

		// Place the leaf values and descend into the loaded child pages.
		next = next[:0]
		for i := range loads {
			load := &loads[i]
			if load.node.page.GetLevel() == 0 {
				values[load.lookups[0].index] = load.value
				found[load.lookups[0].index] = true
				continue
			}
			if child, ok := load.childNode(); ok {
				next = append(next, child)
			}
		}
		level, next = next, level
	}
	return nil
}

// batchLoad is one block the batch walk loads below a node: the value of one
// matched lookup in a leaf, or the child page covering a run of lookups in an
// inner node.
type batchLoad[P walkPage, C any] struct {
	// node is the node holding the entry.
	node *batchNode[P, C]
	// idx is the entry's index in the node's page.
	idx int
	// lookups is the run of lookups under a child page, or the one matched
	// lookup of a value.
	lookups []batchLookup

	// value is the loaded leaf value.
	value []byte
	// page and cursor are the loaded child page and its position.
	page   P
	cursor C
}

// appendBatchLoads appends the loads the lookups under node need: the value of
// each exact non-anchor match in a leaf, or the child page of each run of keys
// sharing a child in an inner node. Keys ordered before the page's first entry
// load nothing.
func appendBatchLoads[P walkPage, C any](loads []batchLoad[P, C], node *batchNode[P, C]) []batchLoad[P, C] {
	page := node.page
	lookups := node.lookups
	for len(lookups) != 0 {
		// Match a leaf's lookups one key at a time.
		idx := page.searchEntry(lookups[0].key)
		if page.GetLevel() == 0 {
			lookup := lookups[:1]
			lookups = lookups[1:]
			if idx < 0 || page.isAnchor(idx) || !bytes.Equal(page.entryKey(idx), lookup[0].key) {
				continue
			}
			loads = append(loads, batchLoad[P, C]{node: node, idx: idx, lookups: lookup})
			continue
		}

		// Take the run of keys under the first remaining key's child page.
		n := 1
		for n < len(lookups) && page.searchEntry(lookups[n].key) == idx {
			n++
		}
		group := lookups[:n]
		lookups = lookups[n:]
		if idx >= 0 {
			loads = append(loads, batchLoad[P, C]{node: node, idx: idx, lookups: group})
		}
	}
	return loads
}

// batchLoadConcurrency bounds the block loads one batch level runs at once.
const batchLoadConcurrency = 16

// runBatchLoads loads each value or child page with at most
// batchLoadConcurrency loads in flight. Each load goes through r, so a block
// the decoded block cache holds costs no store read. Returns the first failure.
func runBatchLoads[P walkPage, C any](ctx context.Context, r pageReader[P, C], loads []batchLoad[P, C]) error {
	// Load a single block on the caller's goroutine.
	if len(loads) == 1 {
		return loads[0].run(ctx, r)
	}

	// Run the loads together; each writes only its own entry. The group's
	// context is its own variable: the closure captures it, and reassigning
	// ctx would move ctx to the heap on the single-load path too.
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(batchLoadConcurrency)
	for i := range loads {
		eg.Go(func() error { return loads[i].run(egCtx, r) })
	}
	return eg.Wait()
}

// run loads one leaf value or child page.
func (l *batchLoad[P, C]) run(ctx context.Context, r pageReader[P, C]) error {
	// Decode a leaf's matched value.
	node := l.node
	if node.page.GetLevel() == 0 {
		value, err := r.entryValue(ctx, node.page, node.cursor, l.idx)
		l.value = value
		return err
	}

	// Load the child page covering the run of lookups.
	page, cursor, err := r.childPage(ctx, node.page, node.cursor, l.idx)
	l.page, l.cursor = page, cursor
	return err
}

// childNode returns the loaded child page with the run of lookups within its
// bounds, and false when none are.
func (l *batchLoad[P, C]) childNode() (batchNode[P, C], bool) {
	group := l.lookups[:0]
	for _, lookup := range l.lookups {
		if l.page.containsKey(lookup.key) {
			group = append(group, lookup)
		}
	}
	return batchNode[P, C]{page: l.page, cursor: l.cursor, lookups: group}, len(group) != 0
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
