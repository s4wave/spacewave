package dot

import (
	"context"
	"os"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/traverse"
)

// PlotToFile plots to an output file.
func PlotToFile(
	ctx context.Context,
	outFilePath string,
	blk block.Block,
	btx *block.Transaction,
	bcs *block.Cursor,
	visitorCb traverse.Visitor,
) error {
	dat, err := Plot(ctx, blk, btx, bcs, visitorCb)
	if err != nil {
		return err
	}
	of, err := os.OpenFile("demo.dot", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer of.Close()
	of.WriteString(string(dat))
	of.WriteString("\n")
	if err := of.Sync(); err != nil {
		return err
	}
	return nil
}
