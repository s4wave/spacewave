package packcopy

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// ReasonUndecodable is a block with unknown refs that its type does not
// decode, so the walk cannot learn its refs.
const ReasonUndecodable = "undecodable"

// Typer decodes blocks by type, so the walk learns the refs of a block whose
// store does not know them. A type is the Typer's own value: the walk only
// carries it from a block to its children.
type Typer interface {
	// RootType returns the type of a root, nil when it has none.
	RootType(ref *block.BlockRef) any
	// Decode decodes data, the verified bytes of ref, as typ. It returns
	// every ref the block holds, each with its type or nil, and ok false when
	// typ cannot say what the block holds. It is called concurrently.
	Decode(ctx context.Context, typ any, ref *block.BlockRef, data []byte) (refs []TypedRef, ok bool, err error)
}

// TypedRef is a ref with the type of the block it names.
type TypedRef struct {
	// Ref is the child.
	Ref *block.BlockRef
	// Type is the child's type, nil when unknown.
	Type any
}

// typeBlock decodes a read block by the node's type. A block whose store knows
// its refs keeps them, and its children gain their types; a block whose refs
// are unknown takes the decoded refs, or stays a hole when its type cannot
// decode it.
func (c *copier) typeBlock(ctx context.Context, n *node, stored *block.StoredBlock) (*block.StoredBlock, map[string]any, string, error) {
	// An untyped block keeps what its store says.
	if c.opts.Typer == nil || n.typ == nil {
		if !stored.RefsKnown {
			return nil, nil, ReasonRefsUnknown, nil
		}
		return stored, nil, "", nil
	}

	// Decode the block by its type.
	typed, ok, err := c.opts.Typer.Decode(ctx, n.typ, n.ref, stored.Data)
	if err != nil {
		return nil, nil, "", err
	}
	if !ok {
		if !stored.RefsKnown {
			return nil, nil, ReasonUndecodable, nil
		}
		return stored, nil, "", nil
	}
	kids := make(map[string]any, len(typed))
	for _, t := range typed {
		if t.Type != nil && !t.Ref.GetEmpty() {
			kids[t.Ref.MarshalString()] = t.Type
		}
	}
	if stored.RefsKnown {
		return stored, kids, "", nil
	}

	// Take the decoded refs in place of the unknown ones.
	refs := make([]*block.BlockRef, 0, len(typed))
	for _, t := range typed {
		if !t.Ref.GetEmpty() {
			refs = append(refs, t.Ref)
		}
	}
	return &block.StoredBlock{Data: stored.Data, Refs: refs, RefsKnown: true}, kids, "", nil
}
