package dot

import (
	"context"
	"os"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/traverse"
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
	// Render the block graph before opening its output file.
	dat, err := Plot(ctx, blk, btx, bcs, visitorCb)
	if err != nil {
		return err
	}

	// Open the requested DOT file, replacing its previous contents.
	of, err := os.OpenFile(outFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	// Write the DOT graph and flush the file to storage.
	_, err = of.Write(append(dat, '\n'))
	if err == nil {
		err = of.Sync()
	}

	// Close the file, reporting the first failure.
	if cerr := of.Close(); err == nil {
		err = cerr
	}
	return err
}
