package traverse

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/pkg/errors"
)

// ErrBreak will terminate visit execution returning a nil error.
var ErrBreak = errors.New("BREAK")

// ErrContinue will skip the node and its children.
var ErrContinue = errors.New("CONTINUE")

// Location is a position in the visitor graph
type Location struct {
	// Depth is the depth of this position.
	Depth int
	// Parent contains the parent location.
	Parent *Location
	// Cursor contains the block graph cursor at the location.
	Cursor *block.Cursor
	// Block contains the block or sub-block at the location.
	// May be nil if the block type is unknown or ref not found.
	Block any
	// ParentRefID is the reference ID that was previously followed.
	ParentRefID uint32
}

// GetParentBlocks traverses the list and gets all parent blocks
// The tail block is the first in the list.
func (l *Location) GetParentBlocks() []any {
	res := make([]any, 0, l.Depth)
	for x := l; x != nil; x = x.Parent {
		res = append(res, x.Block)
	}
	return res
}

// Visitor is the callback for visiting a block graph.
// Returning a non-nil error will end execution.
// Returning ErrBreak will end execution returning a nil error.
type Visitor func(*Location) error

// Visit will walk through a block tree using a depth-first traversal.
// The callback is called with each block in the tree.
//
// If existingOnly, only returns references that have already been traversed.
// If !existingOnly uses GetSubBlocks and/or GetBlockRefs to list all references.
func Visit(
	ctx context.Context,
	blk block.Block,
	bcs *block.Cursor,
	cb Visitor,
	existingOnly bool,
) error {
	loc := &Location{Cursor: bcs, Block: blk}
	err := visitRecursive(ctx, loc, cb, existingOnly)
	if err == ErrBreak || err == ErrContinue {
		return nil
	}
	return err
}

// visitRecursive performs recursive visiting of a tree
//
// TODO: reformat into a stack instead of a recursive func.
func visitRecursive(
	ctx context.Context,
	loc *Location,
	cb Visitor,
	existingOnly bool,
) error {
	// loc is the location to pass to cb() this call
	if (!loc.Cursor.IsSubBlock() && loc.Cursor.GetRef().GetEmpty()) || loc.Block == nil {
		return nil
	}
	if err := cb(loc); err != nil {
		return err
	}
	// follow each ref
	refs, err := loc.Cursor.GetAllRefs(existingOnly)
	if err != nil {
		return errors.Wrap(err, "get block refs")
	}
	locBlock, locBlockOk := loc.Block.(block.BlockWithRefs)
	for refID, refCs := range refs {
		if refCs == nil {
			continue
		}
		refBlk, refBlkIsSubBlk := refCs.GetBlock()
		if refBlk == nil && !refBlkIsSubBlk && locBlockOk {
			blockRefCtor := locBlock.GetBlockRefCtor(refID)
			if blockRefCtor != nil {
				refBlk, err = refCs.Unmarshal(ctx, blockRefCtor)
				if err != nil {
					return errors.Wrapf(err, "follow ref %d", refID)
				}
			}
		}
		err = visitRecursive(ctx, &Location{
			Depth:       loc.Depth + 1,
			Parent:      loc,
			Cursor:      refCs,
			Block:       refBlk,
			ParentRefID: refID,
		}, cb, existingOnly)
		if err != nil {
			if err == ErrContinue {
				continue
			}
			return err
		}
	}
	return nil
}
