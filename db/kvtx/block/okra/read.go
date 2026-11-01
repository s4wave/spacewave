package kvtx_block_okra

import (
	"bytes"
	"context"
	fmt "fmt"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
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
		childCursor := page.FollowChild(pageCursor, idx)
		page, err = loadPage(ctx, childCursor)
		if err != nil {
			return nil, nil, 0, err
		}
		if !page.containsKey(key) {
			return nil, nil, 0, nil
		}
		pageCursor = childCursor
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
type batchNode struct {
	// page is the loaded page.
	page *Page
	// cursor locates page.
	cursor *block.Cursor
	// lookups are the requested keys within the page bounds, sorted by key.
	lookups []batchLookup
}

// findEntriesBatch resolves lookups below page into values and found, indexed
// by each lookup's request position. The lookups are sorted by key, so the keys
// under each child page form one contiguous run and child pages load in key
// order. The walk goes one tree level at a time and reads every block the next
// level needs together before loading them, so a remote store answers the
// whole level in a few round trips. findEntriesBatch reorders lookups in place.
func (t *Tx) findEntriesBatch(
	ctx context.Context,
	page *Page,
	pageCursor *block.Cursor,
	lookups []batchLookup,
	values [][]byte,
	found []bool,
) error {
	level := []batchNode{{page: page, cursor: pageCursor, lookups: lookups}}
	for len(level) != 0 {
		// Read the blocks this level leads to at once, before the walk below
		// loads them one by one.
		prefetchBatch(ctx, level)

		var next []batchNode
		for _, node := range level {
			if node.page.GetLevel() == 0 {
				if err := t.resolveLeafBatch(ctx, node, values, found); err != nil {
					return err
				}
				continue
			}
			var err error
			next, err = appendChildBatch(ctx, next, node)
			if err != nil {
				return err
			}
		}
		level = next
	}
	return nil
}

// resolveLeafBatch decodes the values of the lookups that match entries of the
// leaf node into their request positions.
func (t *Tx) resolveLeafBatch(ctx context.Context, node batchNode, values [][]byte, found []bool) error {
	for _, lookup := range node.lookups {
		// Find an exact non-anchor match for the requested key.
		idx := node.page.searchEntry(lookup.key)
		if idx < 0 {
			continue
		}
		ent := node.page.GetEntries()[idx]
		if ent.GetAnchor() || !bytes.Equal(ent.GetKey(), lookup.key) {
			continue
		}

		// Decode the matched value into its original batch position.
		value, err := t.entryToValue(ctx, node.page, node.cursor, idx)
		if err != nil {
			return err
		}
		values[lookup.index] = value
		found[lookup.index] = true
	}
	return nil
}

// appendChildBatch loads the child pages of the inner node that cover its
// lookups and appends each with the run of lookups within its bounds.
func appendChildBatch(ctx context.Context, next []batchNode, node batchNode) ([]batchNode, error) {
	lookups := node.lookups
	for len(lookups) != 0 {
		// Take the run of keys under the first remaining key's child page.
		idx := node.page.searchEntry(lookups[0].key)
		n := 1
		for n < len(lookups) && node.page.searchEntry(lookups[n].key) == idx {
			n++
		}
		group := lookups[:n]
		lookups = lookups[n:]

		// Skip keys ordered before the page's first entry.
		if idx < 0 {
			continue
		}

		// Load the child page containing this group of requested keys.
		childCursor := node.page.FollowChild(node.cursor, idx)
		childPage, err := loadPage(ctx, childCursor)
		if err != nil {
			return next, err
		}

		// Retain only keys within the loaded child page bounds.
		nextGroup := group[:0]
		for _, lookup := range group {
			if childPage.containsKey(lookup.key) {
				nextGroup = append(nextGroup, lookup)
			}
		}
		if len(nextGroup) != 0 {
			next = append(next, batchNode{page: childPage, cursor: childCursor, lookups: nextGroup})
		}
	}
	return next, nil
}

// batchPrefetchConcurrency bounds the block reads one batch level starts at once.
const batchPrefetchConcurrency = 16

// prefetchBatch reads the blocks the lookups under the level's nodes lead to,
// the child pages of inner nodes or the value blocks of leaves, through the
// store with several reads in flight, so the serial walk that follows finds
// them cached. A remote store answers each miss in one round trip, and the
// walk would otherwise pay those round trips one after another. Failures are
// left for the walk to report.
func prefetchBatch(ctx context.Context, level []batchNode) {
	// Prefetch only through a store.
	store, _ := level[0].cursor.GetBlockStore()
	if store == nil {
		return
	}

	// Collect each distinct block the lookups reach from each node.
	var refs []*block.BlockRef
	for _, node := range level {
		entries := node.page.GetEntries()
		last := -1
		for _, lookup := range node.lookups {
			idx := node.page.searchEntry(lookup.key)
			if idx < 0 || idx == last {
				continue
			}
			last = idx
			ent := entries[idx]
			ref := ent.GetChildRef()
			if node.page.GetLevel() == 0 {
				if ent.GetAnchor() || ent.GetValueBlob() != nil || !bytes.Equal(ent.GetKey(), lookup.key) {
					continue
				}
				ref = ent.GetValueRef()
			}
			if !ref.GetEmpty() {
				refs = append(refs, ref)
			}
		}
	}

	// Read them together when the walk would load several; the walk reports
	// any failure.
	if len(refs) < 2 {
		return
	}
	_ = block.Prefetch(ctx, store, refs, batchPrefetchConcurrency)
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

// rawInlineValue returns the entry's raw inline value, borrowed from the page.
func (p *Page) rawInlineValue(index int) ([]byte, bool) {
	value := p.GetEntries()[index].GetValueBlob()
	if value == nil || value.GetBlobType() != blob.BlobType_BlobType_RAW {
		return nil, false
	}
	return value.GetRawData(), true
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
	data, _, err := valueCursor.Fetch(ctx)
	return data, err
}
