package kvtx_block_okra

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// okraNodeKey identifies one entry position: the anchor or a key.
type okraNodeKey struct {
	// anchor indicates the anchor entry, which has no key.
	anchor bool
	// key is the entry key, nil for the anchor.
	key []byte
}

// okraLevelNode is one entry of a level being rebuilt, with the cursor of the
// child page it references, if any.
type okraLevelNode struct {
	// entry is the node's key entry.
	entry *Entry
	// child is the child page cursor, nil for leaf entries.
	child *block.Cursor
}

// okraPageFrame is one page along a descent from the root, with the cursor it
// was loaded through and the child index used to reach the next frame.
type okraPageFrame struct {
	// page is the loaded page.
	page *Page
	// cursor is the cursor the page was loaded through.
	cursor *block.Cursor
	// index is the child index used to descend to the next frame, -1 for the root.
	index int
}

// okraPagePath is a descent from the root page to a page at the target level.
type okraPagePath []okraPageFrame

// okraBuiltPage is a newly built page with the detached cursor holding it.
type okraBuiltPage struct {
	// page is the built page.
	page *Page
	// cursor is the detached cursor holding the page and its child refs.
	cursor *block.Cursor
}

// newLevelValueNode snapshots the public builder input at the ownership boundary.
func newLevelValueNode(next BuildEntry) (okraLevelNode, error) {
	leafHash, err := hashBuildEntry(next)
	if err != nil {
		return okraLevelNode{}, err
	}
	return okraLevelNode{
		entry: &Entry{
			Key:         slices.Clone(next.Key),
			Hash:        leafHash,
			Size:        1,
			ValueRef:    next.ValueRef.Clone(),
			ValueIsBlob: next.ValueIsBlob,
			ValueBlob:   next.ValueBlob.CloneVT(),
		},
	}, nil
}

// setEntry replaces one leaf entry, rebuilding only the affected page window
// and propagating the parent change upward.
func (t *Tx) setEntry(ctx context.Context, next BuildEntry) error {
	nextNode, err := newLevelValueNode(next)
	if err != nil {
		return err
	}
	key, leafHash := nextNode.entry.Key, nextNode.entry.Hash

	if t.root.GetSize() == 0 {
		return t.setRootFromLevelNodes(ctx, 0, []okraLevelNode{
			{entry: &Entry{Anchor: true, Hash: mustAnchorHash()}},
			nextNode,
		})
	}

	page, _, idx, err := t.findEntry(ctx, key)
	if err != nil {
		return err
	}
	var oldKeys []okraNodeKey
	if page != nil {
		old := page.GetEntries()[idx]
		if bytes.Equal(old.GetHash(), leafHash) {
			return ctx.Err()
		}
		oldKeys = append(oldKeys, entryKey(old))
	}
	return t.replaceLevelEntries(ctx, 0, oldKeys, []okraLevelNode{nextNode})
}

// deleteEntry removes one leaf entry, reporting whether the key was present.
func (t *Tx) deleteEntry(ctx context.Context, key []byte) (bool, error) {
	page, _, idx, err := t.findEntry(ctx, key)
	if err != nil || page == nil {
		return false, err
	}
	if t.root.GetSize() == 1 {
		return true, t.setEmptyRoot(ctx)
	}
	old := page.GetEntries()[idx]
	return true, t.replaceLevelEntries(ctx, 0, []okraNodeKey{entryKey(old)}, nil)
}

// replaceLevelEntries rebuilds the affected windows at one level, then carries
// all their parent changes upward together. Windows stay separate across
// untouched pages. Removing a boundary includes its predecessor; overlapping
// windows merge before any page is built, so a shared ancestor is not rebuilt
// once per affected leaf. The old tree remains intact until the new root is set.
func (t *Tx) replaceLevelEntries(
	ctx context.Context,
	level uint32,
	oldKeys []okraNodeKey,
	replacements []okraLevelNode,
) error {
	if len(oldKeys) == 0 && len(replacements) == 0 {
		return ctx.Err()
	}
	slices.SortFunc(oldKeys, compareNodeKeys)
	slices.SortFunc(replacements, compareLevelNodes)
	locateKeys := make([]okraNodeKey, 0, len(oldKeys)+len(replacements))
	locateKeys = append(locateKeys, oldKeys...)
	for _, node := range replacements {
		locateKeys = append(locateKeys, entryKey(node.entry))
	}
	slices.SortFunc(locateKeys, compareNodeKeys)

	// Each window is an ordered list of paths into the unchanged source tree.
	// A changed page may pull in its predecessor when its boundary disappears.
	var windows [][]okraPagePath
	for _, key := range locateKeys {
		if len(windows) != 0 {
			last := windows[len(windows)-1]
			if pageContainsKey(last[len(last)-1].leaf().page, key) {
				continue
			}
		}
		path, err := t.findPagePath(ctx, level, key)
		if err != nil {
			return err
		}
		page := path.leaf().page
		window := []okraPagePath{path}
		if containsSortedNodeKey(oldKeys, pageStartKey(page)) && !page.GetStartsAtAnchor() {
			previous, ok, err := t.previousPagePath(ctx, path, level)
			if err != nil {
				return err
			}
			if ok {
				window = []okraPagePath{previous, path}
			}
		}
		if len(windows) != 0 {
			last := windows[len(windows)-1]
			if compareNodeKeys(pageStartKey(last[len(last)-1].leaf().page), pageStartKey(window[0].leaf().page)) == 0 {
				windows[len(windows)-1] = append(last, window[1:]...)
				continue
			}
		}
		windows = append(windows, window)
	}

	var oldParentKeys []okraNodeKey
	var parentNodes []okraLevelNode
	nextReplacement := 0
	for _, paths := range windows {
		if err := ctx.Err(); err != nil {
			return err
		}
		var windowNodes []okraLevelNode
		for _, path := range paths {
			frame := path.leaf()
			oldParentKeys = append(oldParentKeys, pageStartKey(frame.page))
			for idx, ent := range frame.page.GetEntries() {
				if containsSortedNodeKey(oldKeys, entryKey(ent)) {
					continue
				}
				var child *block.Cursor
				if level > 0 {
					child = frame.page.FollowChild(frame.cursor, idx)
				}
				windowNodes = append(windowNodes, okraLevelNode{entry: ent, child: child})
			}
		}
		upper := paths[len(paths)-1].leaf().page.GetUpperBound()
		for nextReplacement < len(replacements) {
			node := replacements[nextReplacement]
			if len(upper) != 0 && !node.entry.GetAnchor() && bytes.Compare(node.entry.GetKey(), upper) >= 0 {
				break
			}
			windowNodes = append(windowNodes, node)
			nextReplacement++
		}
		slices.SortFunc(windowNodes, compareLevelNodes)
		if err := validateLevelNodes(windowNodes); err != nil {
			return err
		}
		if level == t.root.GetHeight() {
			// There is one source root page, hence exactly one window at this level.
			return t.setRootFromLevelNodes(ctx, level, windowNodes)
		}
		pages, err := t.buildPagesFromLevelNodes(level, windowNodes, upper)
		if err != nil {
			return err
		}
		parents, err := parentNodesForPages(pages)
		if err != nil {
			return err
		}
		parentNodes = append(parentNodes, parents...)
	}
	return t.replaceLevelEntries(ctx, level+1, oldParentKeys, parentNodes)
}

// findPagePath descends from the root to the page at level containing key.
func (t *Tx) findPagePath(ctx context.Context, level uint32, key okraNodeKey) (okraPagePath, error) {
	if t.root.GetSize() == 0 || level > t.root.GetHeight() {
		return nil, ErrUnexpectedRootMetadata
	}
	page, cursor, err := t.getRootPage(ctx)
	if err != nil {
		return nil, err
	}
	path := okraPagePath{{page: page, cursor: cursor, index: -1}}
	for page.GetLevel() > level {
		idx := 0
		if !key.anchor {
			idx = page.searchEntry(key.key)
		}
		if idx < 0 {
			return nil, ErrUnexpectedPageMetadata
		}
		childCursor := page.FollowChild(cursor, idx)
		child, err := loadPage(ctx, childCursor)
		if err != nil {
			return nil, err
		}
		path = append(path, okraPageFrame{page: child, cursor: childCursor, index: idx})
		page, cursor = child, childCursor
	}
	if page.GetLevel() != level {
		return nil, ErrUnexpectedPageMetadata
	}
	return path, ctx.Err()
}

// previousPagePath returns the path of the page preceding path's leaf at
// level, or false when the leaf is the first page in the tree.
func (t *Tx) previousPagePath(ctx context.Context, path okraPagePath, level uint32) (okraPagePath, bool, error) {
	for depth := len(path) - 2; depth >= 0; depth-- {
		childIdx := path[depth+1].index
		if childIdx <= 0 {
			continue
		}
		return t.descendPagePath(ctx, path[:depth+1], childIdx-1, level, false)
	}
	return nil, false, nil
}

// nextPagePath returns the path of the page following path's leaf at level,
// or false when the leaf is the last page in the tree.
func (t *Tx) nextPagePath(ctx context.Context, path okraPagePath, level uint32) (okraPagePath, bool, error) {
	for depth := len(path) - 2; depth >= 0; depth-- {
		parent := path[depth].page
		childIdx := path[depth+1].index
		if childIdx+1 >= len(parent.GetEntries()) {
			continue
		}
		return t.descendPagePath(ctx, path[:depth+1], childIdx+1, level, true)
	}
	return nil, false, nil
}

// descendPagePath descends from prefix's last frame through childIdx to the
// page at level, taking the first or last child at each deeper level.
func (t *Tx) descendPagePath(
	ctx context.Context,
	prefix okraPagePath,
	childIdx int,
	level uint32,
	first bool,
) (okraPagePath, bool, error) {
	parent := prefix[len(prefix)-1]
	cursor := parent.page.FollowChild(parent.cursor, childIdx)
	page, err := loadPage(ctx, cursor)
	if err != nil {
		return nil, false, err
	}
	out := slices.Clone(prefix)
	out = append(out, okraPageFrame{page: page, cursor: cursor, index: childIdx})
	for page.GetLevel() > level {
		idx := 0
		if !first {
			idx = len(page.GetEntries()) - 1
		}
		cursor = page.FollowChild(cursor, idx)
		page, err = loadPage(ctx, cursor)
		if err != nil {
			return nil, false, err
		}
		out = append(out, okraPageFrame{page: page, cursor: cursor, index: idx})
	}
	return out, true, ctx.Err()
}

// setRootFromLevelNodes builds pages from the nodes and collapses upward until
// a single-entry page above level becomes the new root.
func (t *Tx) setRootFromLevelNodes(ctx context.Context, level uint32, nodes []okraLevelNode) error {
	pages, err := t.buildPagesFromLevelNodes(level, nodes, nil)
	if err != nil {
		return err
	}
	for {
		if len(pages) == 1 && len(pages[0].page.GetEntries()) == 1 && pages[0].page.GetLevel() > 0 {
			return t.setRootPage(ctx, pages[0])
		}
		parentNodes, err := parentNodesForPages(pages)
		if err != nil {
			return err
		}
		level++
		pages, err = t.buildPagesFromLevelNodes(level, parentNodes, nil)
		if err != nil {
			return err
		}
	}
}

// setRootPage installs page's tree as the new root, descending through
// single-entry pages so the root references the deepest single page.
func (t *Tx) setRootPage(ctx context.Context, page okraBuiltPage) error {
	builtRoot := page.cursor
	for page.page.GetLevel() > 1 && len(page.page.GetEntries()) == 1 {
		childCursor := page.page.FollowChild(page.cursor, 0)
		if childCursor == nil {
			return ErrUnexpectedPageMetadata
		}
		childPage, err := loadPage(ctx, childCursor)
		if err != nil {
			return err
		}
		if len(childPage.GetEntries()) != 1 {
			break
		}
		page = okraBuiltPage{page: childPage, cursor: childCursor}
	}
	if page.page.GetSize() == 0 {
		err := t.setEmptyRoot(ctx)
		discardPages(builtRoot)
		return err
	}
	rootEntry := page.page.GetEntries()[0]
	t.replaceRoot(&Root{
		Size:         page.page.GetSize(),
		Height:       page.page.GetLevel(),
		RootHash:     slices.Clone(rootEntry.GetHash()),
		RootPageRef:  page.cursor.GetRef().Clone(),
		HashSize:     HashSize,
		FanoutDegree: FanoutDegree,
	}, page.cursor)
	discardPages(builtRoot)
	return ctx.Err()
}

// setEmptyRoot replaces the root with the empty root metadata.
func (t *Tx) setEmptyRoot(ctx context.Context) error {
	t.replaceRoot(&Root{}, nil)
	return ctx.Err()
}

// replaceRoot releases obsolete pages after attaching their replacement.
// Shared descendants remain attached to the new root or an active iterator.
func (t *Tx) replaceRoot(root *Root, page *block.Cursor) {
	var previous *block.Cursor
	if t.root.GetSize() != 0 {
		previous = t.bcs.FollowRef(rootPageRefID, t.root.GetRootPageRef())
	}
	t.root = root
	t.bcs.ClearAllRefs()
	t.bcs.SetBlock(t.root, true)
	if page != nil {
		t.bcs.SetRef(rootPageRefID, page)
	}
	discardPages(previous)
	if t.rootChangedCb != nil {
		t.rootChangedCb(t.bcs)
	}
}

// discardPages releases only internal pages. Value cursors may outlive their key.
func discardPages(cursor *block.Cursor) {
	for id, child := range cursor.DiscardDetached() {
		if _, ok := entryIndexFromChildRefID(id); ok {
			discardPages(child)
		}
	}
}

// buildPagesFromLevelNodes splits the nodes at page boundaries and builds one
// page per group, using finalUpper as the last page's upper bound.
func (t *Tx) buildPagesFromLevelNodes(level uint32, nodes []okraLevelNode, finalUpper []byte) ([]okraBuiltPage, error) {
	if len(nodes) == 0 {
		return nil, ErrUnexpectedPageMetadata
	}
	pages := make([]okraBuiltPage, 0, 1)
	for start := 0; start < len(nodes); {
		end := start + 1
		for end < len(nodes) && !entryStartsPage(nodes[end].entry) {
			end++
		}
		upper := finalUpper
		if end < len(nodes) {
			upper = nodes[end].entry.GetKey()
		}
		page, err := buildLevelPage(level, nodes[start:end], upper)
		if err != nil {
			return nil, err
		}
		cursor := t.bcs.Detach(false)
		cursor.ClearAllRefs()
		cursor.SetBlock(page, true)
		for idx, node := range nodes[start:end] {
			if node.child != nil {
				cursor.SetRef(entryChildRefID(idx), node.child)
			}
		}
		pages = append(pages, okraBuiltPage{
			page:   page,
			cursor: cursor,
		})
		start = end
	}
	return pages, nil
}

// buildLevelPage builds one page over the nodes with page-local entry storage.
func buildLevelPage(level uint32, nodes []okraLevelNode, upper []byte) (*Page, error) {
	page := &Page{
		Level:      level,
		UpperBound: slices.Clone(upper),
		Entries:    make([]*Entry, len(nodes)),
	}
	for idx, node := range nodes {
		page.Entries[idx] = node.entry
	}
	clonePageEntries(page.Entries)
	if err := refreshPage(page); err != nil {
		return nil, err
	}
	return page, nil
}

// refreshPage recomputes the page's derived metadata and validates it.
func refreshPage(page *Page) error {
	page.StartsAtAnchor = page.GetEntries()[0].GetAnchor()
	page.LowerBound = nil
	page.Size = 0
	for _, ent := range page.GetEntries() {
		if !ent.GetAnchor() && len(page.GetLowerBound()) == 0 {
			page.LowerBound = slices.Clone(ent.GetKey())
		}
		page.Size += ent.GetSize()
	}
	pageHash, err := hashPage(page)
	if err != nil {
		return err
	}
	page.PageHash = pageHash
	return page.Validate()
}

// parentNodesForPages builds one parent entry per page, hashing each page's
// entry range and referencing its cursor.
func parentNodesForPages(pages []okraBuiltPage) ([]okraLevelNode, error) {
	nodes := make([]okraLevelNode, len(pages))
	for idx, page := range pages {
		first := page.page.GetEntries()[0]
		pageHash, err := hashEntryRange(page.page.GetEntries())
		if err != nil {
			return nil, err
		}
		nodes[idx] = okraLevelNode{
			entry: &Entry{
				Anchor:   first.GetAnchor(),
				Key:      slices.Clone(first.GetKey()),
				Hash:     pageHash,
				Size:     page.page.GetSize(),
				ChildRef: page.cursor.GetRef().Clone(),
			},
			child: page.cursor,
		}
	}
	return nodes, nil
}

// leaf returns the deepest frame of the path.
func (p okraPagePath) leaf() okraPageFrame {
	return p[len(p)-1]
}

// validateLevelNodes checks that the nodes are non-empty, non-anchor after the
// first, and strictly sorted.
func validateLevelNodes(nodes []okraLevelNode) error {
	if len(nodes) == 0 {
		return ErrUnexpectedPageMetadata
	}
	for idx := 1; idx < len(nodes); idx++ {
		if nodes[idx].entry.GetAnchor() {
			return ErrUnexpectedEntryMetadata
		}
		if compareEntries(nodes[idx-1].entry, nodes[idx].entry) >= 0 {
			return ErrUnsortedEntries
		}
	}
	return nil
}

// compareLevelNodes orders two level nodes by their entries.
func compareLevelNodes(a, b okraLevelNode) int {
	return compareEntries(a.entry, b.entry)
}

// compareEntries orders two entries by their node keys.
func compareEntries(a, b *Entry) int {
	return compareNodeKeys(entryKey(a), entryKey(b))
}

// compareNodeKeys orders node keys, with the anchor sorting before all keys.
func compareNodeKeys(a, b okraNodeKey) int {
	if a.anchor {
		if b.anchor {
			return 0
		}
		return -1
	}
	if b.anchor {
		return 1
	}
	return bytes.Compare(a.key, b.key)
}

// containsSortedNodeKey reports whether the sorted key slice contains key.
func containsSortedNodeKey(keys []okraNodeKey, key okraNodeKey) bool {
	_, found := slices.BinarySearchFunc(keys, key, compareNodeKeys)
	return found
}

// entryKey returns the node key identifying the entry.
func entryKey(ent *Entry) okraNodeKey {
	if ent.GetAnchor() {
		return okraNodeKey{anchor: true}
	}
	return okraNodeKey{key: ent.GetKey()}
}

// pageStartKey returns the node key of the page's first entry.
func pageStartKey(page *Page) okraNodeKey {
	return entryKey(page.GetEntries()[0])
}

// pageContainsKey reports whether key falls within the page's key range.
func pageContainsKey(page *Page, key okraNodeKey) bool {
	if key.anchor {
		return page.GetStartsAtAnchor()
	}
	if !page.GetStartsAtAnchor() && bytes.Compare(key.key, page.GetLowerBound()) < 0 {
		return false
	}
	if upper := page.GetUpperBound(); len(upper) != 0 && bytes.Compare(key.key, upper) >= 0 {
		return false
	}
	return true
}

// entryStartsPage reports whether the entry begins a new page: it is an
// anchor or its hash is a page boundary.
func entryStartsPage(ent *Entry) bool {
	return ent.GetAnchor() || isBoundary(ent.GetHash())
}

// buildBlobValue materializes val as a blob block and returns its reference.
func (t *Tx) buildBlobValue(ctx context.Context, val []byte) (*block.BlockRef, error) {
	valueCursor := t.buildValueCursor(ctx)
	valueCursor.ClearAllRefs()
	if len(val) == 0 {
		valueCursor.SetBlock(blob.NewBlobBlock(), true)
		return t.materializeValueCursor(ctx, valueCursor)
	}
	if _, err := blob.BuildBlob(ctx, int64(len(val)), bytes.NewReader(val), valueCursor, nil); err != nil {
		return nil, err
	}
	return t.materializeValueCursor(ctx, valueCursor)
}

// buildValueCursor returns a cursor for building a value block against the
// tree's staging store.
func (t *Tx) buildValueCursor(ctx context.Context) *block.Cursor {
	if t.bcs == nil {
		return nil
	}
	return stagedCursor(ctx, t.bcs)
}

// materializeValueCursor writes the cursor's block if dirty and returns its
// reference, adopting staged writes into the owning tree when needed.
func (t *Tx) materializeValueCursor(ctx context.Context, cursor *block.Cursor) (*block.BlockRef, error) {
	if cursor == nil {
		return nil, ctx.Err()
	}
	if cursor.IsDirty() || cursor.GetRef().GetEmpty() {
		btx := cursor.GetTransaction()
		if btx != nil {
			if t.bcs != nil && (cursor.IsSubBlock() || btx == t.bcs.GetTransaction()) {
				// The adopting tree owns staged writes, including inline values
				// borrowed from a different read transaction.
				staged := stagedStore(ctx, t.bcs)
				cursor = cursor.DetachRecursive(true, true, true)
				cursor.MarkDirty()
				btx = cursor.GetTransaction()
				btx.SetStoreOps(staged)
				btx.SetWriteBuffer(staged)
			}
			ref, _, err := btx.WriteAtRoot(ctx, false, cursor)
			if err != nil {
				return nil, err
			}
			return ref.Clone(), nil
		}
	}
	return cursor.GetRef().Clone(), ctx.Err()
}
