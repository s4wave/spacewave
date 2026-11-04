package kvtx_block_okra

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// findReadEntry follows immutable page nodes for a readonly point lookup.
// Writers and iterators retain the mutable path used to propagate changes.
func (t *Tx) findReadEntry(ctx context.Context, key []byte) (*Page, block.ReadCursor, int, error) {
	// Bind this revision's root without comparing it with an earlier revision.
	cursor := *t.readRoot
	page, err := block.UnmarshalReadBlock[Page](ctx, cursor)
	if err != nil {
		return nil, cursor, 0, err
	}

	// Follow only the child page containing the requested key.
	for page.GetLevel() != 0 {
		index := page.searchEntry(key)
		if index < 0 {
			return nil, cursor, 0, nil
		}
		cursor = cursor.FollowRef(entryChildRefID(index), page.GetEntries()[index].GetChildRef())
		page, err = block.UnmarshalReadBlock[Page](ctx, cursor)
		if err != nil {
			return nil, cursor, 0, err
		}
		if !page.containsKey(key) {
			return nil, cursor, 0, nil
		}
	}

	// Require the exact non-anchor leaf entry.
	index := page.searchEntry(key)
	if index < 0 {
		return nil, cursor, 0, nil
	}
	entry := page.GetEntries()[index]
	if entry.GetAnchor() || !bytes.Equal(entry.GetKey(), key) {
		return nil, cursor, 0, nil
	}
	return page, cursor, index, nil
}

// readEntryValue preserves the existing raw, inline and chunked Blob layouts.
func readEntryValue(ctx context.Context, page *Page, cursor block.ReadCursor, index int) ([]byte, error) {
	// Copy an inline raw value without constructing any cursor graph.
	if raw, ok := page.rawInlineValue(index); ok {
		return bytes.Clone(raw), ctx.Err()
	}
	entry := page.GetEntries()[index]

	// Leave inline chunked sub-block traversal on the existing Blob cursor path.
	if entry.GetValueBlob() != nil {
		loaded := cursor.Cursor()
		if _, err := block.UnmarshalBlock[Page](ctx, loaded); err != nil {
			return nil, err
		}
		return blob.FetchToBytes(ctx, page.FollowValue(loaded, index))
	}

	// Read external values under the same transaction and storage authority.
	value := cursor.FollowRef(entryValueRefID(index), entry.GetValueRef())
	if entry.GetValueIsBlob() {
		return blob.FetchToBytes(ctx, value.Cursor())
	}
	data, _, err := value.Fetch(ctx)
	return data, err
}
