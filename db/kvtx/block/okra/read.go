package kvtx_block_okra

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

func loadPage(ctx context.Context, cursor *block.Cursor) (*Page, error) {
	page, err := block.UnmarshalBlock[*Page](ctx, cursor, NewPageBlock)
	if err != nil {
		return nil, err
	}
	if page == nil {
		return nil, block.ErrNotFound
	}
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
	page, pageCursor, err := t.getRootPage(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
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

type batchLookup struct {
	key   []byte
	index int
}

func (t *Tx) findEntriesBatch(
	ctx context.Context,
	page *Page,
	pageCursor *block.Cursor,
	lookups []batchLookup,
	values [][]byte,
	found []bool,
) error {
	if page.GetLevel() == 0 {
		for _, lookup := range lookups {
			idx := page.searchEntry(lookup.key)
			if idx < 0 {
				continue
			}
			ent := page.GetEntries()[idx]
			if ent.GetAnchor() || !bytes.Equal(ent.GetKey(), lookup.key) {
				continue
			}
			value, err := t.entryToValue(ctx, page, pageCursor, idx)
			if err != nil {
				return err
			}
			values[lookup.index] = value
			found[lookup.index] = true
		}
		return nil
	}

	groups := make(map[int][]batchLookup)
	for _, lookup := range lookups {
		idx := page.searchEntry(lookup.key)
		if idx >= 0 {
			groups[idx] = append(groups[idx], lookup)
		}
	}
	for idx, group := range groups {
		childCursor := page.FollowChild(pageCursor, idx)
		childPage, err := loadPage(ctx, childCursor)
		if err != nil {
			return err
		}
		nextGroup := group[:0]
		for _, lookup := range group {
			if childPage.containsKey(lookup.key) {
				nextGroup = append(nextGroup, lookup)
			}
		}
		if len(nextGroup) == 0 {
			continue
		}
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

func (p *Page) searchEntry(key []byte) int {
	idx := -1
	for i, ent := range p.GetEntries() {
		if ent.GetAnchor() {
			idx = i
			continue
		}
		cmp := bytes.Compare(ent.GetKey(), key)
		if cmp > 0 {
			break
		}
		idx = i
		if cmp == 0 {
			break
		}
	}
	return idx
}

func (t *Tx) entryToValue(ctx context.Context, page *Page, cursor *block.Cursor, index int) ([]byte, error) {
	valueCursor := page.FollowValue(cursor, index)
	if page.GetEntries()[index].GetValueIsBlob() {
		return blob.FetchToBytes(ctx, valueCursor)
	}
	data, _, err := valueCursor.Fetch(ctx)
	return data, err
}
