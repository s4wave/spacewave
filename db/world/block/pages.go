package world_block

import (
	"context"
	"slices"
	"strconv"

	"github.com/s4wave/spacewave/db/block"
)

// The kinds the World block types tag their stored blocks with. Every other
// page is untagged and takes its tree from the page above it.
const (
	kindWorld block.BlockKind = iota + 1
	kindObject
	kindChangeLogLL
	// kindObjectTransform is an Object whose root carries a transform
	// config. ObjectRef lists that ref after the root, so it ends the
	// Object's child list.
	kindObjectTransform
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

// WalkUnflushedPages visits each page below the World blocks at roots that
// pages keeps and no open Flush stored, so the next Flush or SetRoots that
// reaches it stores it, once each, with its data length. It walks depth first,
// the object tree before the graph tree and the change log, so a record two
// trees reference counts under its object, and a page two Worlds share counts
// under the first.
//
// The walk follows each kept page's stored child list and types a page by the
// kind its writer tagged it with, so it decodes only the World blocks at
// roots, with xfrm, which may be nil for blocks stored as they are. It ends at
// a page pages does not keep: a stored page's children are stored too, so the
// walk costs one lookup per new page and per ref they hold, not the World's
// size.
func WalkUnflushedPages(
	ctx context.Context,
	pages *block.PendingStore,
	xfrm block.Transformer,
	roots []*block.BlockRef,
	visit func(p Page, size int),
) error {
	// Start from the World blocks, the first on top.
	var stack []pageNode
	seen := make(map[string]struct{})
	for _, root := range slices.Backward(roots) {
		stack = append(stack, pageNode{page: Page{Ref: root, Tree: PageTreeRoot}})
	}
	for len(stack) != 0 {
		// Take the next page not seen yet that pages has yet to store.
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
		b, ok := pages.Unflushed(n.page.Ref)
		if !ok {
			continue
		}

		// The entry before a change log entry sits at its level.
		if n.parent == kindChangeLogLL && b.Kind == kindChangeLogLL {
			n.page.Level--
		}
		visit(n.page, b.Size)

		// Push its children, then reverse them so the first walks first.
		top := len(stack)
		var err error
		stack, err = n.pushChildren(ctx, pages, xfrm, b, stack)
		if err != nil {
			return err
		}
		slices.Reverse(stack[top:])
	}
	return nil
}

// pageNode is a page the walk has yet to visit.
type pageNode struct {
	// page is the page to visit. Its level is one below the page that
	// references it, until the visit places it.
	page Page
	// parent is the kind of the page that references it.
	parent block.BlockKind
}

// pushChildren appends the children of n's page b to stack, first to last.
func (n pageNode) pushChildren(
	ctx context.Context,
	pages *block.PendingStore,
	xfrm block.Transformer,
	b block.UnflushedBlock,
	stack []pageNode,
) ([]pageNode, error) {
	// A World block at a root lists the tree roots inline.
	if n.page.Tree == PageTreeRoot && b.Kind == kindWorld {
		return n.pushWorld(ctx, pages, xfrm, stack)
	}

	// The blocks below an Object's root are its record, and its transform
	// config is not one of its pages. Other pages pass their tree down.
	child := pageNode{page: Page{Tree: n.page.Tree, Level: n.page.Level + 1}, parent: b.Kind}
	refs := b.Refs
	switch b.Kind {
	case kindObjectTransform:
		refs = refs[:max(len(refs)-1, 0)]
		fallthrough
	case kindObject:
		child.page.Tree = PageTreeRecord
	}
	for _, ref := range refs {
		child.page.Ref = ref
		stack = append(stack, child)
	}
	return stack, nil
}

// pushWorld decodes the World block at n and appends the roots of its trees'
// pages to stack, first to last.
func (n pageNode) pushWorld(
	ctx context.Context,
	pages *block.PendingStore,
	xfrm block.Transformer,
	stack []pageNode,
) ([]pageNode, error) {
	// Child lists can't tell the object, graph and change-log tree roots
	// apart, and tagging them would mean changing the World write path.
	data, ok, err := pages.GetBlock(ctx, n.page.Ref)
	if err != nil || !ok {
		return stack, err
	}
	if xfrm != nil {
		if data, err = xfrm.DecodeBlock(slices.Clone(data)); err != nil {
			return stack, err
		}
	}
	w := &World{}
	if err := w.UnmarshalVT(data); err != nil {
		return stack, err
	}

	// Each inline tree root starts its tree at level 1. The inline last
	// change's batch is part of this page, and the stored change log it and
	// prev_changes continue starts at level 1.
	last := w.GetLastChange()
	for _, tree := range [...]struct {
		tree PageTree
		sub  block.SubBlock
	}{
		{PageTreeObject, w.GetObjectKeyValue()},
		{PageTreeGraph, w.GetGraphKeyValue()},
		{PageTreeChangeLog, last.GetChangeBatch()},
	} {
		if tree.sub == nil || tree.sub.IsNil() {
			continue
		}
		refs, err := block.ExtractBlockRefs(tree.sub)
		if err != nil {
			return stack, err
		}
		for _, ref := range refs {
			stack = append(stack, pageNode{page: Page{Ref: ref, Tree: tree.tree, Level: 1}, parent: kindWorld})
		}
	}
	return append(stack,
		pageNode{page: Page{Ref: last.GetPrevRef(), Tree: PageTreeChangeLog, Level: 1}, parent: kindWorld},
		pageNode{page: Page{Ref: w.GetPrevChanges(), Tree: PageTreeChangeLog, Level: 1}, parent: kindWorld},
	), nil
}
