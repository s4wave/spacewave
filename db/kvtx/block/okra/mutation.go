package kvtx_block_okra

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

type okraNodeKey struct {
	anchor bool
	key    []byte
}

type okraLevelNode struct {
	entry *Entry
	child *block.Cursor
}

type okraPageFrame struct {
	page   *Page
	cursor *block.Cursor
	index  int
}

type okraPagePath []okraPageFrame

type okraBuiltPage struct {
	page   *Page
	cursor *block.Cursor
}

func (t *Tx) setEntry(ctx context.Context, next BuildEntry) error {
	key := slices.Clone(next.Key)
	valueRef := next.ValueRef.Clone()
	leafHash, err := hashLeaf(key, valueRef, next.ValueIsBlob)
	if err != nil {
		return err
	}
	nextNode := okraLevelNode{
		entry: &Entry{
			Key:         key,
			Hash:        leafHash,
			Size:        1,
			ValueRef:    valueRef,
			ValueIsBlob: next.ValueIsBlob,
		},
	}

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

func (t *Tx) replaceLevelEntries(
	ctx context.Context,
	level uint32,
	oldKeys []okraNodeKey,
	replacements []okraLevelNode,
) error {
	if len(oldKeys) == 0 && len(replacements) == 0 {
		return ctx.Err()
	}
	locateKeys := make([]okraNodeKey, 0, len(oldKeys)+len(replacements))
	locateKeys = append(locateKeys, oldKeys...)
	for _, node := range replacements {
		locateKeys = append(locateKeys, entryKey(node.entry))
	}
	slices.SortFunc(locateKeys, compareNodeKeys)

	firstPath, err := t.findPagePath(ctx, level, locateKeys[0])
	if err != nil {
		return err
	}
	firstPage := firstPath.leaf().page
	if containsNodeKey(oldKeys, pageStartKey(firstPage)) && !firstPage.GetStartsAtAnchor() {
		prevPath, ok, err := t.previousPagePath(ctx, firstPath, level)
		if err != nil {
			return err
		}
		if ok {
			firstPath = prevPath
		}
	}

	lastKey := locateKeys[len(locateKeys)-1]
	pagePaths := []okraPagePath{firstPath}
	for !pageContainsKey(pagePaths[len(pagePaths)-1].leaf().page, lastKey) {
		nextPath, ok, err := t.nextPagePath(ctx, pagePaths[len(pagePaths)-1], level)
		if err != nil {
			return err
		}
		if !ok {
			return ErrUnexpectedPageMetadata
		}
		pagePaths = append(pagePaths, nextPath)
	}

	windowNodes := make([]okraLevelNode, 0)
	oldParentKeys := make([]okraNodeKey, 0, len(pagePaths))
	for _, path := range pagePaths {
		frame := path.leaf()
		page := frame.page
		oldParentKeys = append(oldParentKeys, pageStartKey(page))
		for idx, ent := range page.GetEntries() {
			if containsNodeKey(oldKeys, entryKey(ent)) {
				continue
			}
			var child *block.Cursor
			if level > 0 {
				child = page.FollowChild(frame.cursor, idx)
			}
			windowNodes = append(windowNodes, okraLevelNode{
				entry: ent,
				child: child,
			})
		}
	}
	windowNodes = append(windowNodes, replacements...)
	slices.SortFunc(windowNodes, compareLevelNodes)
	if err := validateLevelNodes(windowNodes); err != nil {
		return err
	}
	if level == t.root.GetHeight() {
		return t.setRootFromLevelNodes(ctx, level, windowNodes)
	}

	upper := slices.Clone(pagePaths[len(pagePaths)-1].leaf().page.GetUpperBound())
	pages, err := t.buildPagesFromLevelNodes(level, windowNodes, upper)
	if err != nil {
		return err
	}
	parentNodes, err := parentNodesForPages(pages)
	if err != nil {
		return err
	}
	return t.replaceLevelEntries(ctx, level+1, oldParentKeys, parentNodes)
}

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

func (t *Tx) setRootPage(ctx context.Context, page okraBuiltPage) error {
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
		return t.setEmptyRoot(ctx)
	}
	rootEntry := page.page.GetEntries()[0]
	t.root = &Root{
		Size:         page.page.GetSize(),
		Height:       page.page.GetLevel(),
		RootHash:     slices.Clone(rootEntry.GetHash()),
		RootPageRef:  page.cursor.GetRef().Clone(),
		HashSize:     HashSize,
		FanoutDegree: FanoutDegree,
	}
	t.bcs.ClearAllRefs()
	t.bcs.SetBlock(t.root, true)
	t.bcs.SetRef(rootPageRefID, page.cursor)
	if t.rootChangedCb != nil {
		t.rootChangedCb(t.bcs)
	}
	return ctx.Err()
}

func (t *Tx) setEmptyRoot(ctx context.Context) error {
	t.root = &Root{}
	t.bcs.ClearAllRefs()
	t.bcs.SetBlock(t.root, true)
	if t.rootChangedCb != nil {
		t.rootChangedCb(t.bcs)
	}
	return ctx.Err()
}

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

func buildLevelPage(level uint32, nodes []okraLevelNode, upper []byte) (*Page, error) {
	page := &Page{
		Level:      level,
		UpperBound: slices.Clone(upper),
		Entries:    make([]*Entry, len(nodes)),
	}
	for idx, node := range nodes {
		page.Entries[idx] = node.entry.CloneVT()
	}
	if err := refreshPage(page); err != nil {
		return nil, err
	}
	return page, nil
}

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

func (p okraPagePath) leaf() okraPageFrame {
	return p[len(p)-1]
}

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

func compareLevelNodes(a, b okraLevelNode) int {
	return compareEntries(a.entry, b.entry)
}

func compareEntries(a, b *Entry) int {
	return compareNodeKeys(entryKey(a), entryKey(b))
}

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

func containsNodeKey(keys []okraNodeKey, key okraNodeKey) bool {
	return slices.ContainsFunc(keys, func(candidate okraNodeKey) bool {
		return compareNodeKeys(candidate, key) == 0
	})
}

func entryKey(ent *Entry) okraNodeKey {
	if ent.GetAnchor() {
		return okraNodeKey{anchor: true}
	}
	return okraNodeKey{key: ent.GetKey()}
}

func pageStartKey(page *Page) okraNodeKey {
	return entryKey(page.GetEntries()[0])
}

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

func entryStartsPage(ent *Entry) bool {
	return ent.GetAnchor() || isBoundary(ent.GetHash())
}

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

func (t *Tx) buildValueCursor(ctx context.Context) *block.Cursor {
	if t.bcs == nil {
		return nil
	}
	btx := t.bcs.GetTransaction()
	if btx == nil {
		return t.bcs.Detach(false)
	}
	staged := t.stagedValueStore(ctx, btx)
	valueTx, valueCursor := block.NewTransaction(staged, btx.GetTransformer(), nil, btx.GetPutOpts())
	valueTx.SetWriteBuffer(staged)
	return valueCursor
}

func (t *Tx) stagedValueStore(ctx context.Context, btx *block.Transaction) *block.BufferedStore {
	store, _ := t.bcs.GetBlockStore()
	return btx.StageWrites(ctx, valueMaterializationStore(store))
}

type walTrackingStore interface {
	HasWALAppender() bool
	GetStore() block.StoreOps
}

func valueMaterializationStore(store block.StoreOps) block.StoreOps {
	if tracked, ok := store.(walTrackingStore); ok && tracked.HasWALAppender() {
		// The containing Okra page records the value ref; avoid journaling the
		// eager value write while a GC WAL append is in progress.
		return tracked.GetStore()
	}
	return store
}

func (t *Tx) materializeValueCursor(ctx context.Context, cursor *block.Cursor) (*block.BlockRef, error) {
	if cursor == nil {
		return nil, ctx.Err()
	}
	if cursor.IsDirty() || cursor.GetRef().GetEmpty() {
		btx := cursor.GetTransaction()
		if btx != nil {
			if t.bcs != nil && btx == t.bcs.GetTransaction() {
				staged := t.stagedValueStore(ctx, btx)
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
