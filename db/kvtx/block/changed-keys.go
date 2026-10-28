package kvtx_block

import (
	"bytes"
	"context"
	fmt "fmt"
	"slices"
	"sort"

	"github.com/s4wave/spacewave/db/block"
	iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
)

// changedKeysMaxVisits bounds the tree nodes an unrestricted comparison visits.
const changedKeysMaxVisits = 100000

// KeyRange is the half-open key range from Start up to End. A nil End is
// unbounded.
type KeyRange struct {
	// Start is the first key in the range.
	Start []byte
	// End is the first key after the range, or nil for no end.
	End []byte
}

// ExactKeyRange returns the range holding only key.
func ExactKeyRange(key []byte) KeyRange {
	return KeyRange{Start: key, End: append(bytes.Clone(key), 0)}
}

// PrefixKeyRange returns the range of the keys starting with prefix.
func PrefixKeyRange(prefix []byte) KeyRange {
	// Increment the last byte below 0xff after dropping the trailing 0xff
	// bytes; a prefix of only 0xff bytes has no end.
	end := bytes.Clone(prefix)
	for len(end) != 0 && end[len(end)-1] == 0xff {
		end = end[:len(end)-1]
	}
	if len(end) == 0 {
		return KeyRange{Start: prefix}
	}
	end[len(end)-1]++
	return KeyRange{Start: prefix, End: end}
}

// ChangedKeys compares immutable KV roots, skipping equal subtree references.
// It reads tree metadata and inline values, never external value blocks.
// Complete is false after maxKeys changes or 100,000 visited tree nodes;
// callers then reread.
func ChangedKeys(ctx context.Context, before, after *block.Cursor, maxKeys int) (keys [][]byte, complete bool, err error) {
	return changedKeys(ctx, before, after, nil, maxKeys)
}

// ChangedKeysIn lists the keys within ranges that differ between two immutable
// KV roots. It skips every subtree that shares its reference or holds no key in
// ranges, so its cost follows the number of keys in ranges rather than the size
// of the change. ranges may be in any order and may overlap.
func ChangedKeysIn(ctx context.Context, before, after *block.Cursor, ranges []KeyRange) ([][]byte, error) {
	if len(ranges) == 0 {
		return nil, nil
	}
	keys, _, err := changedKeys(ctx, before, after, normalizeKeyRanges(ranges), 0)
	return keys, err
}

// changedKeys compares two KV roots. A nil ranges compares every key, bounded
// by maxKeys and changedKeysMaxVisits; otherwise only keys in the sorted,
// disjoint ranges are compared, without a bound.
func changedKeys(
	ctx context.Context,
	before, after *block.Cursor,
	ranges []KeyRange,
	maxKeys int,
) (keys [][]byte, complete bool, err error) {
	// Open the before and after KV roots as lazy comparison frontiers.
	left, err := newDiffTree(ctx, before)
	if err != nil {
		return nil, false, err
	}
	right, err := newDiffTree(ctx, after)
	if err != nil {
		return nil, false, err
	}

	// Compare the KV frontiers while skipping unchanged subtrees.
	for visited := 0; len(left) != 0 || len(right) != 0; visited++ {
		// Bound the KV comparison by cancellation, node visits, and changed keys.
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if ranges == nil && (visited >= changedKeysMaxVisits || len(keys) >= maxKeys) {
			return nil, false, nil
		}

		// Drop the frontier nodes that cannot hold a key in ranges.
		if ranges != nil && !mayHoldKeyIn(left, ranges) {
			left = left[:len(left)-1]
			continue
		}
		if ranges != nil && !mayHoldKeyIn(right, ranges) {
			right = right[:len(right)-1]
			continue
		}

		// Locate the next nodes on the before and after frontiers.
		var a, b *diffTreeNode
		if len(left) != 0 {
			a = left[len(left)-1]
		}
		if len(right) != 0 {
			b = right[len(right)-1]
		}

		// Skip KV subtrees with identical references and representations.
		if a != nil && b != nil && a.cursor != nil && b.cursor != nil {
			ref := a.cursor.GetRef()
			if !ref.GetEmpty() && ref.EqualsRef(b.cursor.GetRef()) && a.kind == b.kind {
				left, right = left[:len(left)-1], right[:len(right)-1]
				continue
			}
		}

		// Load the frontier nodes needed for the next comparison.
		if err := a.load(ctx); err != nil {
			return nil, false, err
		}
		if err := b.load(ctx); err != nil {
			return nil, false, err
		}

		// Check the ranges again once loading reveals a leaf's key.
		if ranges != nil && (!mayHoldKeyIn(left, ranges) || !mayHoldKeyIn(right, ranges)) {
			continue
		}

		// Collect a leaf that sorts before the other side's subtree, since
		// that subtree and every node after it start after the leaf.
		if a != nil && a.leaf && b != nil && !b.leaf && bytes.Compare(a.key, b.lower) < 0 {
			keys = append(keys, bytes.Clone(a.key))
			left = left[:len(left)-1]
			continue
		}
		if b != nil && b.leaf && a != nil && !a.leaf && bytes.Compare(b.key, a.lower) < 0 {
			keys = append(keys, bytes.Clone(b.key))
			right = right[:len(right)-1]
			continue
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

		// Collect changed leaf keys and advance the ordered frontiers.
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

// normalizeKeyRanges sorts ranges by start and merges overlapping ones.
func normalizeKeyRanges(ranges []KeyRange) []KeyRange {
	// Sort the ranges by their start key.
	sorted := slices.Clone(ranges)
	slices.SortFunc(sorted, func(a, b KeyRange) int {
		return bytes.Compare(a.Start, b.Start)
	})

	// Merge each range into the previous one when they overlap.
	out := sorted[:1]
	for _, r := range sorted[1:] {
		last := &out[len(out)-1]
		if last.End != nil && bytes.Compare(r.Start, last.End) > 0 {
			out = append(out, r)
			continue
		}
		if last.End != nil && (r.End == nil || bytes.Compare(r.End, last.End) > 0) {
			last.End = r.End
		}
	}
	return out
}

// mayHoldKeyIn reports whether the top node of the frontier may hold a key in
// the sorted, disjoint ranges. A node spans from its lower bound up to the
// lower bound of the node below it on the frontier, which follows it in key
// order.
func mayHoldKeyIn(frontier []*diffTreeNode, ranges []KeyRange) bool {
	// Treat an exhausted frontier as holding nothing.
	if len(frontier) == 0 {
		return true
	}

	// Find the first range ending after the node's lower bound.
	n := frontier[len(frontier)-1]
	i := sort.Search(len(ranges), func(i int) bool {
		return ranges[i].End == nil || bytes.Compare(ranges[i].End, n.lower) > 0
	})
	if i == len(ranges) {
		return false
	}

	// Check that the range starts before the node ends.
	if n.leaf {
		return bytes.Compare(ranges[i].Start, n.key) <= 0
	}
	if len(frontier) == 1 {
		return true
	}
	return bytes.Compare(ranges[i].Start, frontier[len(frontier)-2].lower) < 0
}

// diffTreeNode is a lazy ordered frontier; children are stored right-to-left for a stack.
type diffTreeNode struct {
	cursor *block.Cursor
	kind   KVImplType
	loaded bool
	leaf   bool
	height uint32
	// lower is a lower bound of the keys under the node, and a leaf's key
	// once loaded. Every key under a node sorts before the lower bound of the
	// node below it on the frontier.
	lower    []byte
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
	// Ignore absent or previously loaded frontier nodes.
	if n == nil || n.loaded {
		return nil
	}

	// Mark the frontier node loaded and select its representation.
	n.loaded = true
	if n.kind == KVImplType_KV_IMPL_TYPE_IAVL {
		// Read and validate the IAVL frontier node.
		node, err := block.UnmarshalBlock[iavl.Node](ctx, n.cursor)
		if err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("iavl frontier node: %w", block.NotFoundError(n.cursor.GetRef()))
		}
		if err := node.Validate(); err != nil {
			return err
		}

		// Expose the IAVL leaf value or its child references. Keys below an
		// inner node's key go left, so its key bounds the right child.
		n.height = node.GetHeight()
		n.leaf = node.IsLeaf()
		if n.leaf {
			n.key, n.lower = node.GetKey(), node.GetKey()
			n.value, err = node.MarshalVT()
			return err
		}
		n.children = []*diffTreeNode{
			{cursor: n.cursor.FollowRef(6, node.GetRightChildRef()), kind: n.kind, lower: node.GetKey()},
			{cursor: n.cursor.FollowRef(5, node.GetLeftChildRef()), kind: n.kind, lower: n.lower},
		}
		return nil
	}

	// Read the Okra frontier page, which decoding already validated.
	page, err := block.UnmarshalBlock[okra.Page](ctx, n.cursor)
	if err != nil {
		return err
	}
	if page == nil {
		return fmt.Errorf("okra frontier page: %w", block.NotFoundError(n.cursor.GetRef()))
	}

	// Expose Okra leaf values or child references in stack order. A child
	// starts at its entry's key; the level anchor starts where the page does.
	n.height = page.GetLevel() + 1
	entries := page.GetEntries()
	if page.GetLevel() == 0 {
		leaves := make([]diffTreeNode, len(entries))
		for i, entry := range slices.Backward(entries) {
			if !entry.GetAnchor() {
				leaves[i] = diffTreeNode{
					loaded: true,
					leaf:   true,
					lower:  entry.GetKey(),
					key:    entry.GetKey(),
					value:  entry.GetHash(),
				}
				n.children = append(n.children, &leaves[i])
			}
		}
		return nil
	}
	for i, entry := range slices.Backward(entries) {
		if entry.GetChildRef().GetEmpty() {
			continue
		}
		lower := entry.GetKey()
		if entry.GetAnchor() {
			lower = n.lower
		}
		n.children = append(n.children, &diffTreeNode{cursor: page.FollowChild(n.cursor, i), kind: n.kind, lower: lower})
	}
	return nil
}
