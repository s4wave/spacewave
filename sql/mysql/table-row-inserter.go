package mysql

import (
	"context"

	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/dolthub/go-mysql-server/sql"
)

// getPlaceholderValue returns the kvtx placeholder
func getPlaceholderValue() []byte {
	return []byte{0x0}
}

// TableRowInserter implements sql.Inserter against a table.
//
// Note: all table operations are (currently) not concurrency safe.
type TableRowInserter struct {
	ctx           context.Context
	t             *Table
	buildBlobOpts *blob.BuildBlobOpts
}

// NewTableRowInserter constructs a new table row inserter.
func NewTableRowInserter(ctx context.Context, t *Table) sql.RowInserter {
	if ctx == nil {
		ctx = context.Background()
	}
	return &TableRowInserter{
		ctx: ctx,
		t:   t,
	}
}

// SetBuildBlobOpts sets the build blob options.
func (i *TableRowInserter) SetBuildBlobOpts(opts *blob.BuildBlobOpts) {
	i.buildBlobOpts = opts
}

// Insert inserts the row given, returning an error if it cannot. Insert will be
// called once for each row to process for the insert operation, which may
// involve many rows. After all rows in an operation have been processed, Close
// is called.
func (i *TableRowInserter) Insert(sqlCtx *sql.Context, row sql.Row) error {
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}
	schema := i.t.schema
	if len(row) != len(schema) {
		return sql.ErrInvalidColumnNumber.New(len(schema), len(row))
	}
	nnonce := i.t.root.RowNonce
	pt, _, err := i.t.SelectPartition(nnonce)
	if err != nil {
		return err
	}
	rowKey := MarshalTableRowKey(nnonce)
	tx, err := pt.BuildTreeTx(false)
	if err != nil {
		return err
	}
	// set to a dummy value
	// TODO SetWithCursor
	err = tx.Set(rowKey, getPlaceholderValue(), 0)
	if err != nil {
		return err
	}
	// get the cursor at the location in the tree
	_, rowCursor, err := tx.GetWithCursor(rowKey)
	if err != nil {
		return err
	}
	tpr := &TablePartitionRow{RowNonce: nnonce}
	rowCursor.ClearAllRefs()
	rowCursor.SetBlock(tpr, true)
	trCursor := rowCursor.FollowRef(2, nil)
	tableRow, err := BuildTableRow(cctx, trCursor, row, i.buildBlobOpts)
	if err != nil {
		return err
	}
	trCursor.SetBlock(tableRow, true)
	i.t.root.RowNonce++
	return nil
}

// Close closes the table row inserter.
func (i *TableRowInserter) Close(sqlCtx *sql.Context) error {
	// TODO
	return nil
}

// _ is a type assertion
var _ sql.RowInserter = ((*TableRowInserter)(nil))
