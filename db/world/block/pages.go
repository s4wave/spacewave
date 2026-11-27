package world_block

import (
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
	// Start from the World blocks, the first on top.
	seen := make(map[string]struct{})
	var stack []pageNode
	for _, root := range slices.Backward(roots) {
		stack = append(stack, pageNode{ctor: NewWorldBlock, page: Page{Ref: root, Tree: PageTreeRoot}})
	}
	for len(stack) != 0 {
		// Take the next page not seen yet, and ask whether to enter it.
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.page.Ref.GetEmpty() {
			continue
		}
		h := n.page.Ref.GetHash()
		key := string(rune(h.GetHashType())) + string(h.GetHash())
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		enter, err := visit(n.page)
		if err != nil {
			return err
		}
		if !enter {
			continue
		}

		// Read its children, pushing them last first so the first walks
		// first.
		children, err := n.children(ctx, store, xfrm)
		if err != nil {
			return err
		}
		for _, child := range slices.Backward(children) {
			stack = append(stack, child)
		}
	}
	return nil
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

// children reads the page and returns its children.
func (n pageNode) children(ctx context.Context, store block.StoreOps, xfrm block.Transformer) ([]pageNode, error) {
	// Read the page with its stored child list.
	sb, err := store.GetStoredBlock(ctx, n.page.Ref)
	if err != nil || sb == nil {
		return nil, err
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
				return n.typedChildren(blk), nil
			}
		}
	}

	// Walk any other page by its stored child list.
	if !sb.RefsSource.Recorded() {
		return nil, nil
	}
	out := make([]pageNode, 0, len(sb.Refs))
	for _, ref := range sb.Refs {
		out = append(out, n.child(ref, nil, nil))
	}
	return out, nil
}

// typedChildren returns the children blk references, typed as its refs declare
// them. Sub-blocks are part of the page, so their refs are the page's
// children.
func (n pageNode) typedChildren(blk any) []pageNode {
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
		return slices.Concat(
			object.subBlockChildren(b.GetObjectKeyValue()),
			graph.subBlockChildren(b.GetGraphKeyValue()),
			changes.subBlockChildren(last.GetChangeBatch()),
			[]pageNode{
				changes.child(last.GetPrevRef(), NewChangeLogLLBlock, nil),
				changes.child(b.GetPrevChanges(), NewChangeLogLLBlock, nil),
			},
		)
	case *ChangeLogLL:
		// The change log is a list, so the entry before this one sits at its
		// level, and only the entry's batch hangs below it.
		prev := n
		prev.page.Level--
		return append(
			[]pageNode{prev.child(b.GetPrevRef(), NewChangeLogLLBlock, nil)},
			n.subBlockChildren(b.GetChangeBatch())...,
		)
	case *Object:
		// An Object's root is its record, untyped here: the walk reads no
		// object type. A root in another bucket is not a ref of this page.
		root := b.GetRootRef()
		if root.GetBucketId() != "" {
			return nil
		}
		record := pageNode{page: Page{Tree: PageTreeRecord, Level: n.page.Level}}
		return []pageNode{record.child(root.GetRootRef(), nil, nil)}
	case *kvtx_block.KeyValueStore, *kvtx_block_iavl.Node, *kvtx_block_okra.Root, *kvtx_block_okra.Page:
	default:
		// Only tree pages pass the tree's values down.
		n.values = nil
	}

	// Type each ref as its holder declares it, or as the tree's value.
	var out []pageNode
	if holder, ok := blk.(block.BlockWithRefs); ok {
		refs, _ := holder.GetBlockRefs()
		for _, id := range slices.Sorted(maps.Keys(refs)) {
			ctor := holder.GetBlockRefCtor(id)
			if ctor == nil {
				ctor = kvtx_block.TreeValueCtor(blk, id, n.values)
			}
			out = append(out, n.child(refs[id], ctor, n.values))
		}
	}

	// Add the refs of each inline sub-block.
	if holder, ok := blk.(block.BlockWithSubBlocks); ok {
		subs := holder.GetSubBlocks()
		for _, id := range slices.Sorted(maps.Keys(subs)) {
			out = append(out, n.subBlockChildren(subs[id])...)
		}
	}
	return out
}

// subBlockChildren returns the children of an inline sub-block of n's page,
// none for an absent one.
func (n pageNode) subBlockChildren(sub block.SubBlock) []pageNode {
	if sub == nil || sub.IsNil() {
		return nil
	}
	return n.typedChildren(sub)
}

// child returns the node of ref one level below n, in n's tree.
func (n pageNode) child(ref *block.BlockRef, ctor, values block.Ctor) pageNode {
	return pageNode{
		page:   Page{Ref: ref, Tree: n.page.Tree, Level: n.page.Level + 1},
		ctor:   ctor,
		values: values,
	}
}
