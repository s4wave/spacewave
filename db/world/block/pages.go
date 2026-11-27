package world_block

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strconv"

	"github.com/s4wave/spacewave/db/block"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
)

// PageTree identifies the part of a World a page belongs to.
type PageTree int

const (
	// PageTreeRoot is the World block itself, with the tree roots inline in it.
	PageTreeRoot PageTree = iota
	// PageTreeObject holds the object tree's nodes and the Object blocks
	// its values reference.
	PageTreeObject
	// PageTreeRecord holds the blocks below an Object's root: its record.
	PageTreeRecord
	// PageTreeGraph holds the graph tree's pages and values.
	PageTreeGraph
	// PageTreeChangeLog holds the change log's entries and change batches.
	PageTreeChangeLog
)

// String returns the tree's metric label.
func (t PageTree) String() string {
	switch t {
	case PageTreeRoot:
		return "root"
	case PageTreeObject:
		return "object"
	case PageTreeRecord:
		return "record"
	case PageTreeGraph:
		return "graph"
	case PageTreeChangeLog:
		return "changelog"
	}
	return "tree" + strconv.Itoa(int(t))
}

// Page is a block below a World root.
type Page struct {
	// Ref addresses the block.
	Ref *block.BlockRef
	// Tree is the part of the World that holds the block.
	Tree PageTree
	// Level counts the blocks between this one and the World block, which
	// is level 0. A tree root inline in the World block is part of it, so
	// the pages that root references are level 1.
	Level int
}

// WalkPages visits each page below the World blocks at roots once, depth first,
// the object tree before the graph tree and the change log, so a record two
// trees reference counts under its object, and a page two Worlds share counts
// under the first. It reads each page it enters from store and decodes it with
// xfrm, which may be nil for blocks stored as they are.
//
// visit reports whether to walk below a page, before the walk reads it. A
// caller counting the pages a store adds returns false for a page the store
// holds already: a stored page's children are stored too, so the walk reads
// only the new pages and costs what they cost, not the World's size.
//
// A typed page is decoded and its children typed by the refs it declares. The
// blocks below an Object's root, and any ref the holder leaves untyped, are
// walked by the child list the store recorded, and not walked when it recorded
// none. A page that does not decode is walked the same way.
func WalkPages(
	ctx context.Context,
	store block.StoreOps,
	xfrm block.Transformer,
	roots []*block.BlockRef,
	visit func(Page) (bool, error),
) error {
	var w pageWalk
	return w.walk(ctx, store, xfrm, roots, visit)
}

// walk runs WalkPages with w's buffers.
func (w *pageWalk) walk(
	ctx context.Context,
	store block.StoreOps,
	xfrm block.Transformer,
	roots []*block.BlockRef,
	visit func(Page) (bool, error),
) error {
	// Start from the World blocks, the first on top.
	seen := make(map[string]struct{})
	for _, root := range slices.Backward(roots) {
		w.stack = append(w.stack, pageNode{ctor: NewWorldBlock, page: Page{Ref: root, Tree: PageTreeRoot}})
	}
	for len(w.stack) != 0 {
		// Take the next page not seen yet, and ask whether to enter it.
		n := w.stack[len(w.stack)-1]
		w.stack = w.stack[:len(w.stack)-1]
		enter, err := n.enter(seen, visit)
		if err != nil {
			return err
		}
		if !enter {
			continue
		}

		// Push its children, then reverse them so the first walks first.
		top := len(w.stack)
		if err := w.pushChildren(ctx, store, xfrm, n); err != nil {
			return err
		}
		slices.Reverse(w.stack[top:])
	}
	return nil
}

// pageWalk holds the buffers a walk reuses from page to page, so typing a page
// builds no slice of its children and no map of its sub-blocks.
type pageWalk struct {
	// stack holds the pages yet to visit, the next on top.
	stack []pageNode
	// subs holds the sub-blocks of the blocks being typed: each block's in
	// one region above its holder's, sorted by id.
	subs []idSubBlock
}

// idSubBlock is a sub-block with the id its holder gives it.
type idSubBlock struct {
	id  uint32
	sub block.SubBlock
}

// pageNode is a page the walk has yet to visit, with what types its children.
type pageNode struct {
	// page is the page to visit.
	page Page
	// ctor constructs the page's type, nil for a page walked by its stored
	// child list.
	ctor block.Ctor
	// values constructs the values of the tree the page belongs to, nil when
	// the page is not a tree node or the values are untyped.
	values block.Ctor
}

// enter skips empty or previously seen pages and asks visit whether to walk
// below a newly seen page, recording it before the callback runs.
func (n pageNode) enter(seen map[string]struct{}, visit func(Page) (bool, error)) (bool, error) {
	// Identify and record the first visit to a nonempty page.
	if n.page.Ref.GetEmpty() {
		return false, nil
	}
	h := n.page.Ref.GetHash()
	key := string(rune(h.GetHashType())) + string(h.GetHash())
	if _, ok := seen[key]; ok {
		return false, nil
	}
	seen[key] = struct{}{}
	return visit(n.page)
}

// pushChildren reads n's page and pushes its children, first to last.
func (w *pageWalk) pushChildren(ctx context.Context, store block.StoreOps, xfrm block.Transformer, n pageNode) error {
	// Read the page with its stored child list.
	sb, err := store.GetStoredBlock(ctx, n.page.Ref)
	if err != nil || sb == nil {
		return err
	}

	// Decode a typed page, from a copy the decode cannot overwrite in the
	// store's memory.
	if n.ctor != nil {
		data := sb.Data
		if xfrm != nil {
			data, err = xfrm.DecodeBlock(slices.Clone(data))
		}
		if err == nil {
			blk := n.ctor()
			if blk.UnmarshalVT(data) == nil {
				w.pushTyped(n, blk)
				return nil
			}
		}
	}

	// Walk any other page by its stored child list.
	if !sb.RefsSource.Recorded() {
		return nil
	}
	for _, ref := range sb.Refs {
		w.stack = append(w.stack, n.child(ref, nil, nil))
	}
	return nil
}

// pushTyped pushes the children blk references, typed as its refs declare
// them. Sub-blocks are part of the page, so their refs are the page's
// children.
func (w *pageWalk) pushTyped(n pageNode, blk any) {
	// List the children of the World and Object blocks, which set the tree
	// below them, and keep the tree's values only on its own pages.
	switch b := blk.(type) {
	case *World:
		// Each inline tree root starts its tree. The inline last change's
		// batch is part of this page, and the stored change log it and
		// prev_changes continue starts at level 1.
		object := pageNode{page: Page{Tree: PageTreeObject}, values: NewObjectBlock}
		graph := pageNode{page: Page{Tree: PageTreeGraph}}
		changes := pageNode{page: Page{Tree: PageTreeChangeLog}}
		last := b.GetLastChange()
		w.pushSubBlock(object, b.GetObjectKeyValue())
		w.pushSubBlock(graph, b.GetGraphKeyValue())
		w.pushSubBlock(changes, last.GetChangeBatch())
		w.stack = append(w.stack,
			changes.child(last.GetPrevRef(), NewChangeLogLLBlock, nil),
			changes.child(b.GetPrevChanges(), NewChangeLogLLBlock, nil),
		)
		return
	case *ChangeLogLL:
		// The change log is a list, so the entry before this one sits at its
		// level, and only the entry's batch hangs below it.
		prev := n
		prev.page.Level--
		w.stack = append(w.stack, prev.child(b.GetPrevRef(), NewChangeLogLLBlock, nil))
		w.pushSubBlock(n, b.GetChangeBatch())
		return
	case *Object:
		// An Object's root is its record, untyped here: the walk reads no
		// object type. A root in another bucket is not a ref of this page.
		root := b.GetRootRef()
		if root.GetBucketId() != "" {
			return
		}
		record := pageNode{page: Page{Tree: PageTreeRecord, Level: n.page.Level}}
		w.stack = append(w.stack, record.child(root.GetRootRef(), nil, nil))
		return
	case *kvtx_block.KeyValueStore, *kvtx_block_iavl.Node, *kvtx_block_okra.Root, *kvtx_block_okra.Page:
	default:
		// Only tree pages pass the tree's values down.
		n.values = nil
	}

	// Type each ref as its holder declares it, or as the tree's value.
	if holder, ok := blk.(block.BlockWithRefs); ok {
		refs, _ := holder.GetBlockRefs()
		for _, id := range slices.Sorted(maps.Keys(refs)) {
			ctor := holder.GetBlockRefCtor(id)
			if ctor == nil {
				ctor = kvtx_block.TreeValueCtor(blk, id, n.values)
			}
			w.stack = append(w.stack, n.child(refs[id], ctor, n.values))
		}
	}

	// List the inline sub-blocks in a region of w.subs, sorted by id.
	holder, ok := blk.(block.BlockWithSubBlocks)
	if !ok {
		return
	}
	base := len(w.subs)
	if ranger, ok := blk.(block.BlockWithSubBlockRange); ok {
		ranger.RangeSubBlocks(func(id uint32, sub block.SubBlock) {
			w.subs = append(w.subs, idSubBlock{id: id, sub: sub})
		})
	} else {
		for id, sub := range holder.GetSubBlocks() {
			w.subs = append(w.subs, idSubBlock{id: id, sub: sub})
		}
	}
	top := len(w.subs)
	slices.SortFunc(w.subs[base:top], func(x, y idSubBlock) int { return cmp.Compare(x.id, y.id) })

	// Add the refs of each. A sub-block's own sub-blocks take the region
	// above top and may move w.subs, so read each entry by index.
	for i := base; i < top; i++ {
		w.pushSubBlock(n, w.subs[i].sub)
	}
	clear(w.subs[base:top])
	w.subs = w.subs[:base]
}

// pushSubBlock pushes the children of an inline sub-block of n's page, none
// for an absent one.
func (w *pageWalk) pushSubBlock(n pageNode, sub block.SubBlock) {
	if sub == nil || sub.IsNil() {
		return
	}
	w.pushTyped(n, sub)
}

// child returns the node of ref one level below n, in n's tree.
func (n pageNode) child(ref *block.BlockRef, ctor, values block.Ctor) pageNode {
	return pageNode{
		page:   Page{Ref: ref, Tree: n.page.Tree, Level: n.page.Level + 1},
		ctor:   ctor,
		values: values,
	}
}
