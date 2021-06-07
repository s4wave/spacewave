package mysql

import (
	"context"

	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/util/ival"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/pkg/errors"
)

// getPlaceholderValue returns the kvtx placeholder
func getPlaceholderValue() []byte {
	return []byte{0x0}
}

// TableEditor implements row management operations against a table.
//
// Note: all table operations are (currently) not concurrency safe.
type TableEditor struct {
	ctx           context.Context
	t             *Table
	buildBlobOpts *blob.BuildBlobOpts
}

// NewTableEditor constructs a new table row inserter.
func NewTableEditor(ctx context.Context, t *Table) *TableEditor {
	if ctx == nil {
		ctx = context.Background()
	}
	return &TableEditor{
		ctx: ctx,
		t:   t,
	}
}

// SetBuildBlobOpts sets the build blob options.
func (i *TableEditor) SetBuildBlobOpts(opts *blob.BuildBlobOpts) {
	i.buildBlobOpts = opts
}

// StatementBegin is called before the first operation of a statement.
// Integrators should mark the state of the data in some way that it may be
// returned to in the case of an error.
func (i *TableEditor) StatementBegin(ctx *sql.Context) {
	// TODO mark state so we can return to it later (Discard)
	// really we need a wrapper for this, which creates a new TableEditorTx each time.
	return
}

// Insert inserts the row given, returning an error if it cannot. Insert will be
// called once for each row to process for the insert operation, which may
// involve many rows. After all rows in an operation have been processed, Close
// is called.
func (i *TableEditor) Insert(sqlCtx *sql.Context, row sql.Row) error {
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

	// TODO: check Primary Key collision
	// if another row with the same primary key exists:
	// return sql.ErrPrimaryKeyViolation.New(fmt.Sprint(vals))

	// auto increment
	autoIncrIdx := i.t.autoIncrIdx
	schemaCols := i.t.schema
	if autoIncrIdx != 0 {
		autoIncrIdx-- // 1-based index
		// ensure next Insert() auto_increment is at least this row + 1
		autoIncrVal := i.t.autoIncrVal
		if autoIncrIdx >= len(schemaCols) {
			return errors.Errorf("auto increment index out of range: %d > %d", autoIncrIdx, len(schemaCols)-1)
		}
		autoIncrCol := schemaCols[autoIncrIdx]
		cmp, err := autoIncrCol.Type.Compare(row[autoIncrIdx], autoIncrVal)
		if err != nil {
			return errors.Wrap(err, "auto increment type mismatch")
		}
		if cmp > 0 {
			autoIncrVal = row[autoIncrIdx]
		}
		autoIncrVal = ival.Increment(autoIncrVal)
		err = i.SetAutoIncrementValue(sqlCtx, autoIncrVal)
		if err != nil {
			return err
		}
	}

	rowKey := MarshalTableRowKey(nnonce)
	tx, err := pt.BuildTreeTx(false)
	if err != nil {
		return err
	}
	// set to a dummy value
	// NOTE SetWithCursor would make this cleaner
	err = tx.Set(rowKey, getPlaceholderValue())
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
	i.t.bcs.SetBlock(i.t.root, true)
	return nil
}

// SetAutoIncrementValue sets a new AUTO_INCREMENT value.
func (i *TableEditor) SetAutoIncrementValue(sqlCtx *sql.Context, val interface{}) error {
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}
	bcs := i.t.bcs.FollowSubBlock(4)
	var err error
	i.t.root.AutoIncrVal, err = BuildTableColumn(cctx, bcs, i.buildBlobOpts, val)
	if err != nil {
		return err
	}
	i.t.bcs.SetBlock(i.t.root, true)
	return nil
}

// DiscardChanges is called if a statement encounters an error, and all current
// changes since the statement beginning should be discarded.
func (i *TableEditor) DiscardChanges(ctx *sql.Context, errorEncountered error) error {
	return errors.New("TODO DiscardChanges in table editor")
}

// StatementComplete is called after the last operation of the statement,
// indicating that it has successfully completed. The mark set in StatementBegin
// may be removed, and a new one should be created on the next StatementBegin.
func (i *TableEditor) StatementComplete(ctx *sql.Context) error {
	// TODO
	return nil
}

// Close finalizes the operation, persisting its result.
func (i *TableEditor) Close(sqlCtx *sql.Context) error {
	// TODO: is it necessary to wait to apply until Close() ?
	return nil
}

// _ is a type assertion
var (
	_ sql.AutoIncrementSetter = ((*TableEditor)(nil))
	_ sql.RowInserter         = ((*TableEditor)(nil))
)
