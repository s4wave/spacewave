package dot

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/traverse"
	"gonum.org/v1/gonum/graph/encoding/dot"
)

// Plot plots a block structure, traversing references.
// visitorCb can be nil
func Plot(
	ctx context.Context,
	blk block.Block,
	btx *block.Transaction,
	bcs *block.Cursor,
	visitorCb traverse.Visitor,
) ([]byte, error) {
	// Fill the btx with the contents of the traversal.
	err := traverse.Visit(
		ctx,
		blk, bcs,
		func(loc *traverse.Location) error {
			if visitorCb != nil {
				if err := visitorCb(loc); err != nil {
					return err
				}
			}

			return nil
		},
		false,
	)
	if err != nil {
		return nil, err
	}

	// Build a overlay graph.Graph structure with extra info.
	// This extra info implements the dot interfaces

	// Use the block graph.
	return dot.Marshal(
		btx.GetBlockGraph(),
		bcs.GetRef().MarshalString(),
		"", "",
	)
}
