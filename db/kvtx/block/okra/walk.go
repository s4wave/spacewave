package kvtx_block_okra

import (
	"context"
	"fmt"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// walkPage is a decoded page a walk reads: *Page on the mutable cursor tree,
// or *flatPage on a ReadCursor.
type walkPage interface {
	// GetLevel returns the page height; zero is a leaf.
	GetLevel() uint32
	// Len returns the number of entries.
	Len() int
	// isAnchor reports whether entry i is the anchor.
	isAnchor(i int) bool
	// entryKey returns the key of entry i, borrowed from the page.
	entryKey(i int) []byte
	// searchEntry returns the index of the last entry at or before key.
	searchEntry(key []byte) int
	// containsKey reports whether key falls within the page bounds.
	containsKey(key []byte) bool
	// rawInlineValue returns entry i's raw inline value, borrowed from the page.
	rawInlineValue(i int) ([]byte, bool)
}

// pageReader loads one kind of page and the values its entries hold. Page P
// was loaded through position C.
//
// A writer, or a read over pages staged in this transaction, reads through
// cursorPages: its pages live on the mutable cursor tree, which keeps every
// page it loads until the transaction ends, because a commit walks the edited
// path back to the root and a staged page exists only on its parent's edge. A
// read of stored pages reads through flatPages: a ReadCursor keeps no parent
// graph, so a walk holds only the pages its caller holds, and the decoded-block
// cache bounds the rest.
type pageReader[P walkPage, C any] interface {
	// rootPage loads the tree's root page.
	rootPage(ctx context.Context) (P, C, error)
	// childPage loads the child page of entry index of page.
	childPage(ctx context.Context, page P, cursor C, index int) (P, C, error)
	// entryValue returns an owned copy of entry index's value.
	entryValue(ctx context.Context, page P, cursor C, index int) ([]byte, error)
	// valueCursor returns a cursor at entry index's value.
	valueCursor(ctx context.Context, page P, cursor C, index int) (*block.Cursor, error)
}

// cursorPages reads a transaction's pages through its mutable cursor tree.
type cursorPages struct{ t *Tx }

// rootPage loads the root page through the transaction's root cursor.
func (r cursorPages) rootPage(ctx context.Context) (*Page, *block.Cursor, error) {
	return r.t.getRootPage(ctx)
}

// childPage follows the entry's child edge in the cursor tree.
func (r cursorPages) childPage(ctx context.Context, page *Page, cursor *block.Cursor, index int) (*Page, *block.Cursor, error) {
	child := page.FollowChild(cursor, index)
	out, err := loadPage(ctx, child)
	return out, child, err
}

// entryValue returns an owned copy of the entry's value.
func (r cursorPages) entryValue(ctx context.Context, page *Page, cursor *block.Cursor, index int) ([]byte, error) {
	return r.t.entryToValue(ctx, page, cursor, index)
}

// valueCursor follows the entry's value edge in the cursor tree.
func (r cursorPages) valueCursor(_ context.Context, page *Page, cursor *block.Cursor, index int) (*block.Cursor, error) {
	return page.FollowValue(cursor, index), nil
}

// flatPages reads a read transaction's stored pages through ReadCursors.
type flatPages struct{ t *Tx }

// rootPage loads the stored root page.
func (r flatPages) rootPage(ctx context.Context) (*flatPage, block.ReadCursor, error) {
	cursor := *r.t.readRoot
	page, err := loadFlatPage(ctx, cursor)
	return page, cursor, err
}

// childPage loads the entry's stored child page.
func (r flatPages) childPage(ctx context.Context, page *flatPage, cursor block.ReadCursor, index int) (*flatPage, block.ReadCursor, error) {
	child := page.followChild(cursor, index)
	out, err := loadFlatPage(ctx, child)
	return out, child, err
}

// entryValue returns an owned copy of the entry's value.
func (r flatPages) entryValue(ctx context.Context, page *flatPage, cursor block.ReadCursor, index int) ([]byte, error) {
	return readEntryValue(ctx, page, cursor, index)
}

// valueCursor materializes a private cursor at the entry's value. An inline
// Blob is a sub-block of its page, so its page is decoded onto that cursor.
func (r flatPages) valueCursor(ctx context.Context, page *flatPage, cursor block.ReadCursor, index int) (*block.Cursor, error) {
	// A stored value needs no page cursor.
	if !page.hasInlineBlob(index) {
		return page.followValue(cursor, index).Cursor(), nil
	}

	// Decode the page onto a private cursor to reach the inline Blob.
	loaded := cursor.Cursor()
	mutable, err := loadPage(ctx, loaded)
	if err != nil {
		return nil, err
	}
	return mutable.FollowValue(loaded, index), nil
}

// loadFlatPage returns the validated flat page stored at cursor. A reader
// with no block, which an empty child reference follows, is a missing page.
func loadFlatPage(ctx context.Context, cursor block.ReadCursor) (*flatPage, error) {
	page, err := block.UnmarshalReadBlock[flatPage](ctx, cursor)
	if err == nil && page == nil {
		err = errMissingPage
	}
	return page, err
}

// errMissingPage reports a page reference that names no block.
var errMissingPage = fmt.Errorf("okra page: %w", block.ErrNotFound)

// pageFrame is one page along a descent from the root, with the position it
// was loaded through and the child index used to reach it.
type pageFrame[P walkPage, C any] struct {
	// page is the loaded page.
	page P
	// cursor is the position the page was loaded through.
	cursor C
	// index is the parent's entry this page is the child of, -1 for the root.
	index int
}

// pagePath is a descent from the root page to a page at the target level.
type pagePath[P walkPage, C any] []pageFrame[P, C]

// cursorPath is a descent through the mutable cursor tree, as writers walk it.
type cursorPath = pagePath[*Page, *block.Cursor]

// leaf returns the deepest frame of the path.
func (p pagePath[P, C]) leaf() pageFrame[P, C] {
	return p[len(p)-1]
}

// findPagePath descends from the root to the page at level containing key.
func findPagePath[P walkPage, C any](ctx context.Context, r pageReader[P, C], root *Root, level uint32, key okraNodeKey) (pagePath[P, C], error) {
	// Require a populated tree with the requested page level.
	if root.GetSize() == 0 || level > root.GetHeight() {
		return nil, ErrUnexpectedRootMetadata
	}

	// Load the root page before descending toward the requested level.
	page, cursor, err := r.rootPage(ctx)
	if err != nil {
		return nil, err
	}

	// Record the page path while following the child that contains the key.
	path := pagePath[P, C]{{page: page, cursor: cursor, index: -1}}
	for page.GetLevel() > level {
		idx := 0
		if !key.anchor {
			idx = page.searchEntry(key.key)
		}
		if idx < 0 {
			return nil, ErrUnexpectedPageMetadata
		}
		page, cursor, err = r.childPage(ctx, page, cursor, idx)
		if err != nil {
			return nil, err
		}
		path = append(path, pageFrame[P, C]{page: page, cursor: cursor, index: idx})
	}

	// Verify that the descent reached the requested page level.
	if page.GetLevel() != level {
		return nil, ErrUnexpectedPageMetadata
	}
	return path, ctx.Err()
}

// lastPagePath descends from the root to the last page at level.
func lastPagePath[P walkPage, C any](ctx context.Context, r pageReader[P, C], root *Root, level uint32) (pagePath[P, C], error) {
	// Require a populated tree with the requested page level.
	if root.GetSize() == 0 || level > root.GetHeight() {
		return nil, ErrUnexpectedRootMetadata
	}

	// Load the root page, then take the last child down to the level.
	page, cursor, err := r.rootPage(ctx)
	if err != nil {
		return nil, err
	}
	path := pagePath[P, C]{{page: page, cursor: cursor, index: -1}}
	if page.GetLevel() > level {
		return descendPagePath(ctx, r, path, page.Len()-1, level, false)
	}

	// Verify that the root is the requested level.
	if page.GetLevel() != level {
		return nil, ErrUnexpectedPageMetadata
	}
	return path, ctx.Err()
}

// previousPagePath returns the path of the page preceding path's leaf at
// level, or false when the leaf is the first page in the tree.
func previousPagePath[P walkPage, C any](ctx context.Context, r pageReader[P, C], path pagePath[P, C], level uint32) (pagePath[P, C], bool, error) {
	for depth := len(path) - 2; depth >= 0; depth-- {
		childIdx := path[depth+1].index
		if childIdx <= 0 {
			continue
		}
		out, err := descendPagePath(ctx, r, path[:depth+1], childIdx-1, level, false)
		return out, err == nil, err
	}
	return nil, false, nil
}

// nextPagePath returns the path of the page following path's leaf at level,
// or false when the leaf is the last page in the tree.
func nextPagePath[P walkPage, C any](ctx context.Context, r pageReader[P, C], path pagePath[P, C], level uint32) (pagePath[P, C], bool, error) {
	for depth := len(path) - 2; depth >= 0; depth-- {
		childIdx := path[depth+1].index
		if childIdx+1 >= path[depth].page.Len() {
			continue
		}
		out, err := descendPagePath(ctx, r, path[:depth+1], childIdx+1, level, true)
		return out, err == nil, err
	}
	return nil, false, nil
}

// descendPagePath descends from prefix's last frame through childIdx to the
// page at level, taking the first or last child at each deeper level. The
// result shares no frames with prefix.
func descendPagePath[P walkPage, C any](
	ctx context.Context,
	r pageReader[P, C],
	prefix pagePath[P, C],
	childIdx int,
	level uint32,
	first bool,
) (pagePath[P, C], error) {
	// Load the selected child below the existing path prefix.
	parent := prefix.leaf()
	page, cursor, err := r.childPage(ctx, parent.page, parent.cursor, childIdx)
	if err != nil {
		return nil, err
	}

	// Extend the path through the first or last child at each deeper level.
	out := slices.Clone(prefix)
	out = append(out, pageFrame[P, C]{page: page, cursor: cursor, index: childIdx})
	for page.GetLevel() > level {
		idx := 0
		if !first {
			idx = page.Len() - 1
		}
		page, cursor, err = r.childPage(ctx, page, cursor, idx)
		if err != nil {
			return nil, err
		}
		out = append(out, pageFrame[P, C]{page: page, cursor: cursor, index: idx})
	}
	if page.GetLevel() != level {
		return nil, ErrUnexpectedPageMetadata
	}
	return out, ctx.Err()
}

// Len returns the number of entries.
func (p *Page) Len() int { return len(p.GetEntries()) }

// isAnchor reports whether entry i is the anchor.
func (p *Page) isAnchor(i int) bool { return p.GetEntries()[i].GetAnchor() }

// entryKey returns the key of entry i, borrowed from the page.
func (p *Page) entryKey(i int) []byte { return p.GetEntries()[i].GetKey() }

// rawInlineValue returns the entry's raw inline value, borrowed from the page.
func (p *Page) rawInlineValue(index int) ([]byte, bool) {
	value := p.GetEntries()[index].GetValueBlob()
	if value == nil || value.GetBlobType() != blob.BlobType_BlobType_RAW {
		return nil, false
	}
	return value.GetRawData(), true
}
