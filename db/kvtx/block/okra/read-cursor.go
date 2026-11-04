package kvtx_block_okra

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
)

// findReadEntry follows immutable page nodes for a readonly point lookup.
// Writers and iterators retain the mutable path used to propagate changes. The
// returned page is flat: it borrows its key and value bytes from its buffer.
func (t *Tx) findReadEntry(ctx context.Context, key []byte) (*flatPage, block.ReadCursor, int, error) {
	// Bind this revision's root without comparing it with an earlier revision.
	cursor := *t.readRoot
	page, err := block.UnmarshalReadBlock[flatPage](ctx, cursor)
	if err != nil {
		return nil, cursor, 0, err
	}

	// Follow only the child page containing the requested key.
	for page.Level() != 0 {
		index := page.searchEntry(key)
		if index < 0 {
			return nil, cursor, 0, nil
		}
		cursor = page.followChild(cursor, index)
		page, err = block.UnmarshalReadBlock[flatPage](ctx, cursor)
		if err != nil {
			return nil, cursor, 0, err
		}
		if !page.containsKey(key) {
			return nil, cursor, 0, nil
		}
	}

	// Require the exact non-anchor leaf entry.
	index := page.searchEntry(key)
	if index < 0 || page.isAnchor(index) || !bytes.Equal(page.entryKey(index), key) {
		return nil, cursor, 0, nil
	}
	return page, cursor, index, nil
}

// followChild returns a reader at entry index's child page.
func (p *flatPage) followChild(cursor block.ReadCursor, index int) block.ReadCursor {
	return followFlatRef(cursor, entryChildRefID(index), p, p.entries[index].child)
}

// followValue returns a reader at entry index's external value.
func (p *flatPage) followValue(cursor block.ReadCursor, index int) block.ReadCursor {
	return followFlatRef(cursor, entryValueRefID(index), p, p.entries[index].value)
}

// followFlatRef returns a reader at the block r names in page p. An empty
// reference follows nil, as a decoded page's nil reference does.
func followFlatRef(cursor block.ReadCursor, id uint32, p *flatPage, r flatRef) block.ReadCursor {
	if r.empty() {
		return cursor.FollowRef(id, nil)
	}
	return cursor.FollowDigest(id, hash.HashType(r.hashType), p.view(r.digest))
}

// readEntryValue preserves the existing raw, inline and chunked Blob layouts.
func readEntryValue(ctx context.Context, page *flatPage, cursor block.ReadCursor, index int) ([]byte, error) {
	// Copy an inline raw value without constructing any cursor graph.
	if raw, ok := page.rawInlineValue(index); ok {
		return bytes.Clone(raw), ctx.Err()
	}

	// Leave inline chunked sub-block traversal on the existing Blob cursor path.
	if page.hasInlineBlob(index) {
		loaded := cursor.Cursor()
		mutable, err := block.UnmarshalBlock[Page](ctx, loaded)
		if err != nil {
			return nil, err
		}
		return blob.FetchToBytes(ctx, mutable.FollowValue(loaded, index))
	}

	// Read external values under the same transaction and storage authority.
	value := page.followValue(cursor, index)
	if page.valueIsBlob(index) {
		return blob.FetchToBytes(ctx, value.Cursor())
	}
	data, _, err := value.Fetch(ctx)
	return data, err
}
