// Package order groups related blocks for physical packfile locality.
package order

import (
	"cmp"
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ChildLists reads the child list a Volume keeps for each stored block.
type ChildLists interface {
	// GetChildren returns the children of ref and whether its list is known.
	GetChildren(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, bool, error)
}

// BlockRefs groups refs by available subtrees, read from lists. Child lists
// have no intrinsic order, so siblings use stable hash order. Without lists,
// all refs use stable hash order. Only candidate blocks are queried.
func BlockRefs(ctx context.Context, lists ChildLists, refs []*block.BlockRef) ([]*block.BlockRef, error) {
	// Deduplicate candidate identities before reading their lists.
	ordered := NewGraph()
	for _, ref := range refs {
		ordered.Add(ref, nil)
	}
	if lists == nil {
		return ordered.Order(ctx, nil)
	}
	keys := make([]string, 0, len(ordered.nodes))
	for key := range ordered.nodes {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	// Capture each candidate's children in stable hash order.
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ref := ordered.nodes[key].ref
		children, _, err := lists.GetChildren(ctx, ref)
		if err != nil {
			return nil, errors.Wrap(err, "get child list")
		}
		children = slices.Clone(children)
		slices.SortFunc(children, func(a, b *block.BlockRef) int {
			return cmp.Compare(refKey(a), refKey(b))
		})
		ordered.Add(ref, children)
	}
	return ordered.Order(ctx, nil)
}

// refKey identifies a nonempty block by its content hash.
func refKey(ref *block.BlockRef) string {
	if ref == nil || ref.GetEmpty() || ref.GetHash() == nil {
		return ""
	}
	return ref.GetHash().MarshalString()
}
