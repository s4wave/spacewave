package kvtx_block

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
)

// ChangedKeys compares immutable KV roots, skipping equal subtree references.
// It reads tree metadata and inline values, never external value blocks. Complete
// is false after maxKeys changes or 100,000 visited tree nodes; callers then reread.
func ChangedKeys(ctx context.Context, before, after *block.Cursor, maxKeys int) (keys [][]byte, complete bool, err error) {
	left, err := newDiffTree(ctx, before)
	if err != nil {
		return nil, false, err
	}
	right, err := newDiffTree(ctx, after)
	if err != nil {
		return nil, false, err
	}
	for visited := 0; len(left) != 0 || len(right) != 0; visited++ {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if visited >= 100000 || len(keys) >= maxKeys {
			return nil, false, nil
		}
		var a, b *diffTreeNode
		if len(left) != 0 {
			a = left[len(left)-1]
		}
		if len(right) != 0 {
			b = right[len(right)-1]
		}
		if a != nil && b != nil && a.cursor != nil && b.cursor != nil {
			ref := a.cursor.GetRef()
			if !ref.GetEmpty() && ref.EqualsRef(b.cursor.GetRef()) && a.kind == b.kind {
				left, right = left[:len(left)-1], right[:len(right)-1]
				continue
			}
		}
		if err := a.load(ctx); err != nil {
			return nil, false, err
		}
		if err := b.load(ctx); err != nil {
			return nil, false, err
		}

		// Expand the taller frontier until both sides expose comparable leaves.
		if a != nil && !a.leaf && (b == nil || b.leaf || a.height >= b.height) {
			left = append(left[:len(left)-1], a.children...)
			continue
		}
		if b != nil && !b.leaf {
			right = append(right[:len(right)-1], b.children...)
			continue
		}
		if a == nil || b != nil && bytes.Compare(b.key, a.key) < 0 {
			keys = append(keys, bytes.Clone(b.key))
			right = right[:len(right)-1]
		} else if b == nil || bytes.Compare(a.key, b.key) < 0 {
			keys = append(keys, bytes.Clone(a.key))
			left = left[:len(left)-1]
		} else {
			if !bytes.Equal(a.value, b.value) {
				keys = append(keys, bytes.Clone(a.key))
			}
			left, right = left[:len(left)-1], right[:len(right)-1]
		}
	}
	return keys, true, nil
}

// diffTreeNode is a lazy ordered frontier; children are stored right-to-left for a stack.
type diffTreeNode struct {
	cursor   *block.Cursor
	kind     KVImplType
	loaded   bool
	leaf     bool
	height   uint32
	key      []byte
	value    []byte
	children []*diffTreeNode
}

// newDiffTree opens only the root of the selected KV representation.
func newDiffTree(ctx context.Context, cursor *block.Cursor) ([]*diffTreeNode, error) {
	root, err := LoadKeyValueStore(ctx, cursor)
	if err != nil {
		return nil, err
	}
	switch root.GetImplType() {
	case KVImplType_KV_IMPL_TYPE_IAVL:
		if root.GetIavlRoot().GetSize() == 0 {
			return nil, nil
		}
		return []*diffTreeNode{{cursor: cursor.FollowSubBlock(2), kind: root.GetImplType()}}, nil
	case KVImplType_KV_IMPL_TYPE_OKRA, KVImplType_KV_IMPL_TYPE_OKRA_INLINE:
		if root.GetOkraRoot().GetSize() == 0 {
			return nil, nil
		}
		return []*diffTreeNode{{cursor: cursor.FollowSubBlock(3).FollowRef(4, root.GetOkraRoot().GetRootPageRef()), kind: root.GetImplType()}}, nil
	default:
		return nil, NewErrUnknownImpl(root.GetImplType())
	}
}

// load reads one frontier node and exposes children without reading their blocks.
func (n *diffTreeNode) load(ctx context.Context) error {
	if n == nil || n.loaded {
		return nil
	}
	n.loaded = true
	if n.kind == KVImplType_KV_IMPL_TYPE_IAVL {
		node, err := block.UnmarshalBlock[*iavl.Node](ctx, n.cursor, iavl.NewNodeBlock)
		if err != nil {
			return err
		}
		if node == nil {
			return block.ErrNotFound
		}
		if err := node.Validate(); err != nil {
			return err
		}
		n.height = node.GetHeight()
		n.leaf = node.IsLeaf()
		if n.leaf {
			n.key = node.GetKey()
			n.value, err = node.MarshalVT()
			return err
		}
		n.children = []*diffTreeNode{
			{cursor: n.cursor.FollowRef(6, node.GetRightChildRef()), kind: n.kind},
			{cursor: n.cursor.FollowRef(5, node.GetLeftChildRef()), kind: n.kind},
		}
		return nil
	}

	page, err := block.UnmarshalBlock[*okra.Page](ctx, n.cursor, okra.NewPageBlock)
	if err != nil {
		return err
	}
	if page == nil {
		return block.ErrNotFound
	}
	if err := page.Validate(); err != nil {
		return err
	}
	n.height = page.GetLevel() + 1
	entries := page.GetEntries()
	for i, entry := range slices.Backward(entries) {
		if page.GetLevel() == 0 {
			if !entry.GetAnchor() {
				n.children = append(n.children, &diffTreeNode{loaded: true, leaf: true, key: entry.GetKey(), value: entry.GetHash()})
			}
		} else if !entry.GetChildRef().GetEmpty() {
			n.children = append(n.children, &diffTreeNode{cursor: page.FollowChild(n.cursor, i), kind: n.kind})
		}
	}
	return nil
}
