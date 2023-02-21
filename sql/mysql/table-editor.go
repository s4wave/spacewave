package mysql

import (
	"context"

	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
	"github.com/pkg/errors"
)

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
	schema := i.t.schema.Schema
	if len(row) != len(schema) {
		return sql.ErrInvalidColumnNumber.New(len(schema), len(row))
	}
	rowNonce := i.t.root.RowNonce
	pt, _, err := i.t.SelectPartition(rowNonce)
	if err != nil {
		return err
	}

	// TODO: check Primary Key collision
	// if another row with the same primary key(s) exists:
	// return sql.ErrPrimaryKeyViolation.New(fmt.Sprint(vals))
	// TODO: may require accessing the table index for the primary key(s)

	// auto increment
	autoIncIdx := i.t.autoIncIdx
	schemaCols := i.t.schema.Schema
	if autoIncIdx != 0 {
		autoIncIdx-- // 1-based index
		// ensure next Insert() auto_increment is at least this row + 1
		autoIncVal := i.t.autoIncVal
		if autoIncIdx >= len(schemaCols) {
			return errors.Errorf("auto increment index out of range: %d > %d", autoIncIdx, len(schemaCols)-1)
		}
		autoIncCol := schemaCols[autoIncIdx]
		cmp, err := autoIncCol.Type.Compare(row[autoIncIdx], autoIncVal)
		if err != nil {
			return errors.Wrap(err, "auto increment type mismatch")
		}
		if cmp > 0 {
			// Provided value larger than autoIncVal, set autoIncVal to that value
			v, err := types.Uint64.Convert(row[autoIncIdx])
			if err != nil {
				return errors.Wrap(err, "auto increment type mismatch")
			}
			autoIncVal = v.(uint64)
			autoIncVal++ // Move onto next autoIncVal
		} else if cmp == 0 {
			autoIncVal++
		}

		err = i.SetAutoIncrementValue(sqlCtx, autoIncVal)
		if err != nil {
			return err
		}
	}

	rowKey := MarshalTableRowKey(rowNonce)
	tx, err := pt.BuildTreeTx(i.ctx, false, true)
	if err != nil {
		return err
	}
	rootCursor := tx.GetCursor()

	// detach the root cursor to create a cursor for the new TableRow.
	rowCursor := rootCursor.Detach(false)
	rowCursor.ClearAllRefs()
	_, err = BuildTableRow(cctx, rowCursor, row, i.buildBlobOpts)
	if err != nil {
		return err
	}

	// set the row to the rowKey
	err = tx.SetCursorAtKey(rowKey, rowCursor, false)
	if err != nil {
		return err
	}

	// increment the row nonce
	i.t.root.RowNonce++
	i.t.bcs.SetBlock(i.t.root, true)
	return nil
}

// SetAutoIncrementValue sets a new AUTO_INCREMENT value.
func (i *TableEditor) SetAutoIncrementValue(sqlCtx *sql.Context, val uint64) error {
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}
	err := i.t.root.StoreAutoIncrVal(cctx, i.t.bcs, i.buildBlobOpts, val)
	if err != nil {
		return err
	}
	i.t.autoIncVal = val
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
