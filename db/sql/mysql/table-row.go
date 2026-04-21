package mysql

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/block/sbset"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/pkg/errors"
)

// BuildTableRow constructs a TableRow by marshaling cols with msgpack blobs.
//
// Stores sub-blocks and references using the cursor. Sets the block at the cursor.
// If columns are large enough, they will be sharded into separate blocks.
// Supports streaming decoding at a later time.
// buildBlobOpts is optional
// bcs is required
// autoIncIdx and autoIncVal are optional.
func BuildTableRow(
	ctx context.Context,
	bcs *block.Cursor,
	row sql.Row,
	buildBlobOpts *blob.BuildBlobOpts,
) (*TableRow, error) {
	tr := &TableRow{}
	tr.Columns = make([]*TableColumn, len(row))
	var err error
	var colSet *sbset.SubBlockSet
	bcs.ClearAllRefs()
	bcs.SetBlock(tr, true)
	colSet = newTableRowColumnSetContainer(tr, bcs)
	for i, col := range row {
		// follow sub-block for the column
		_, ibcs := colSet.Get(i)
		tr.Columns[i], err = BuildTableColumn(ctx, ibcs, buildBlobOpts, col)
		if err != nil {
			return nil, errors.Wrapf(err, "column[%d]", i)
		}
	}
	return tr, nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*TableRow)(nil))
	_ block.BlockWithSubBlocks = ((*TableRow)(nil))
)
