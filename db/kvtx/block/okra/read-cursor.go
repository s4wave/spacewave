package kvtx_block_okra

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
)

// flatPos is where a read found a flat page: the reader of the block that
// stores it, and the page, which is the block's root page or a page embedded
// in it. A position whose page is nil names a block not yet loaded.
type flatPos struct {
	// cursor reads the block that stores the page.
	cursor block.ReadCursor
	// page is the page, or nil until a load reads the block's root page.
	page *flatPage
}

// at returns the position with page, the page loaded through it. A walk keeps
// the position childAt returned, which names a stored block by its cursor
// alone until a load, so each step that reads the page pins it first.
func (pos flatPos) at(page *flatPage) flatPos {
	pos.page = page
	return pos
}

// load returns the position's page, loading the block's root page when the
// position names a block not yet loaded.
func (pos flatPos) load(ctx context.Context) (flatPos, error) {
	if pos.page != nil {
		return pos, nil
	}
	page, err := loadFlatPage(ctx, pos.cursor)
	return flatPos{cursor: pos.cursor, page: page}, err
}

// findReadEntry follows immutable page nodes for a readonly point lookup.
// Writers and staged reads use findEntry on the mutable cursor tree. The
// returned page is flat: it borrows its key and value bytes from its buffer.
func (t *Tx) findReadEntry(ctx context.Context, key []byte) (flatPos, int, error) {
	// Bind this revision's root without comparing it with an earlier revision.
	pos, err := flatPos{cursor: *t.readRoot}.load(ctx)
	if err != nil {
		return flatPos{}, 0, err
	}

	// Follow only the child page containing the requested key.
	for pos.page.GetLevel() != 0 {
		index := pos.page.searchEntry(key)
		if index < 0 {
			return flatPos{}, 0, nil
		}
		pos, err = pos.child(index).load(ctx)
		if err != nil {
			return flatPos{}, 0, err
		}
		if !pos.page.containsKey(key) {
			return flatPos{}, 0, nil
		}
	}

	// Require the exact non-anchor leaf entry.
	page := pos.page
	index := page.searchEntry(key)
	if index < 0 || page.isAnchor(index) || !bytes.Equal(page.entryKey(index), key) {
		return flatPos{}, 0, nil
	}
	return pos, index, nil
}

// child returns the position of entry index's child page: the page itself
// when the entry embeds it, or the block the entry's ref names.
func (pos flatPos) child(index int) flatPos {
	p := pos.page
	if child := p.embeddedChild(index); child != nil {
		return flatPos{cursor: pos.cursor, page: child}
	}
	return flatPos{cursor: p.followChildRef(pos.cursor, index)}
}

// followChildRef returns a reader at the block entry index's child ref names.
func (p *flatPage) followChildRef(cursor block.ReadCursor, index int) block.ReadCursor {
	return followFlatRef(cursor, entryChildRefID(index), p, p.entries[index].child)
}

// followValue returns a reader at entry index's external value.
func (p *flatPage) followValue(cursor block.ReadCursor, index int) block.ReadCursor {
	return followFlatRef(cursor, entryValueRefID(index), p, p.entries[index].value)
}

// followFlatRef returns a reader at the block r references in page p. An empty
// reference follows nil, as a decoded page's nil reference does.
func followFlatRef(cursor block.ReadCursor, id uint32, p *flatPage, r flatRef) block.ReadCursor {
	if r.empty() {
		return cursor.FollowRef(id, nil)
	}
	return cursor.FollowDigest(id, hash.HashType(r.hashType), p.view(r.digest))
}

// readEntryValue preserves the existing raw, inline and chunked Blob layouts.
func readEntryValue(ctx context.Context, pos flatPos, index int) ([]byte, error) {
	// Copy an inline raw value without constructing any cursor graph.
	page := pos.page
	if raw, ok := page.rawInlineValue(index); ok {
		return bytes.Clone(raw), ctx.Err()
	}

	// Leave inline chunked sub-block traversal on the existing Blob cursor path.
	if page.hasInlineBlob(index) {
		value, err := pos.inlineBlobCursor(ctx, index)
		if err != nil {
			return nil, err
		}
		return blob.FetchToBytes(ctx, value)
	}

	// Read external values under the same transaction and storage authority.
	value := page.followValue(pos.cursor, index)
	if page.valueIsBlob(index) {
		return blob.FetchToBytes(ctx, value.Cursor())
	}
	data, _, err := value.FetchLeaf(ctx)
	return data, err
}

// valueCursor returns a private cursor at entry index's value.
func (pos flatPos) valueCursor(ctx context.Context, index int) (*block.Cursor, error) {
	if !pos.page.hasInlineBlob(index) {
		return pos.page.followValue(pos.cursor, index).Cursor(), nil
	}
	return pos.inlineBlobCursor(ctx, index)
}

// inlineBlobCursor returns a private cursor at entry index's inline Blob. The
// Blob is a sub-block of its page, so the block's pages are decoded onto a
// private cursor down the page's route to reach it.
func (pos flatPos) inlineBlobCursor(ctx context.Context, index int) (*block.Cursor, error) {
	page, cursor, err := pos.page.decodeOnto(ctx, pos.cursor.Cursor())
	if err != nil {
		return nil, err
	}
	return page.FollowValue(cursor, index), nil
}

// decodeOnto decodes the pages of the block at cursor down p's route, and
// returns p as a decoded Page with the cursor that holds it.
func (p *flatPage) decodeOnto(ctx context.Context, cursor *block.Cursor) (*Page, *block.Cursor, error) {
	page, err := loadPage(ctx, cursor)
	if err != nil {
		return nil, nil, err
	}
	for _, idx := range p.route {
		cursor = page.FollowChild(cursor, int(idx))
		if page, err = loadPage(ctx, cursor); err != nil {
			return nil, nil, err
		}
	}
	return page, cursor, nil
}
