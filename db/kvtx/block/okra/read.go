package kvtx_block_okra

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

func loadPage(ctx context.Context, cursor *block.Cursor) (*Page, error) {
	// Decode the Okra page stored at the supplied cursor.
	page, err := block.UnmarshalBlock[*Page](ctx, cursor, NewPageBlock)
	if err != nil {
		return nil, err
	}

	// Require a decoded page before validating its metadata.
	if page == nil {
		return nil, block.ErrNotFound
	}

	// Validate the decoded Okra page before returning it.
	if err := page.Validate(); err != nil {
		return nil, err
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

// findEntriesBatch resolves lookups below page into values and found, indexed
// by each lookup's request position. Child pages load in key order, so a batch
// reads the same blocks in the same order every time.
func (t *Tx) findEntriesBatch(
	ctx context.Context,
	page *Page,
	pageCursor *block.Cursor,
	lookups []batchLookup,
	values [][]byte,
	found []bool,
) error {
	// Resolve requested values directly when the page is a leaf.
	if page.GetLevel() == 0 {
		for _, lookup := range lookups {
			// Find an exact non-anchor match for the requested key.
			idx := page.searchEntry(lookup.key)
			if idx < 0 {
				continue
			}
			ent := page.GetEntries()[idx]
			if ent.GetAnchor() || !bytes.Equal(ent.GetKey(), lookup.key) {
				continue
			}

			// Decode the matched value into its original batch position.
			value, err := t.entryToValue(ctx, page, pageCursor, idx)
			if err != nil {
				return err
			}
			values[lookup.index] = value
			found[lookup.index] = true
		}
		return nil
	}

	// Group requested keys by the child page that covers them.
	groups := make([][]batchLookup, len(page.GetEntries()))
	for _, lookup := range lookups {
		idx := page.searchEntry(lookup.key)
		if idx >= 0 {
			groups[idx] = append(groups[idx], lookup)
		}
	}

	// Resolve each populated child group in page order.
	for idx, group := range groups {
		// Skip child pages with no requested keys.
		if len(group) == 0 {
			continue
		}

		// Load the child page containing this group of requested keys.
		childCursor := page.FollowChild(pageCursor, idx)
		childPage, err := loadPage(ctx, childCursor)
		if err != nil {
			return err
		}

		// Retain only keys within the loaded child page bounds.
		nextGroup := group[:0]
		for _, lookup := range group {
			if childPage.containsKey(lookup.key) {
				nextGroup = append(nextGroup, lookup)
			}
		}
		if len(nextGroup) == 0 {
			continue
		}

		// Resolve the retained keys beneath the child page.
		if err := t.findEntriesBatch(ctx, childPage, childCursor, nextGroup, values, found); err != nil {
			return err
		}
	}
	return nil
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
