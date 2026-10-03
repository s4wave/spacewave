package mysql

import (
	"bytes"
	"context"
	"io"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/blob"
)

// TableEditor implements row management operations against a table.
//
// Note: all table operations are (currently) not concurrency safe.
type TableEditor struct {
	ctx           context.Context
	t             *Table
	buildBlobOpts *blob.BuildBlobOpts
	statementRoot *TableRoot
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
	i.statementRoot = i.t.root.CloneVT()
}

// Insert inserts the row given, returning an error if it cannot. Insert will be
// called once for each row to process for the insert operation, which may
// involve many rows. After all rows in an operation have been processed, Close
// is called.
func (i *TableEditor) Insert(sqlCtx *sql.Context, row sql.Row) error {
	// Use the SQL operation context for row access.
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}

	// Provide a SQL context for schema and constraint checks.
	checkCtx := sqlCtx
	if checkCtx == nil {
		checkCtx = sql.NewContext(cctx)
	}

	// Require the inserted row to match the table column count.
	schema := i.t.schema.Schema
	if len(row) != len(schema) {
		return sql.ErrInvalidColumnNumber.New(len(schema), len(row))
	}

	// Select the partition for the next table row.
	rowNonce := i.t.root.RowNonce
	pt, _, err := i.t.SelectPartition(rowNonce)
	if err != nil {
		return err
	}

	// Check the inserted row against primary and unique keys.
	if err := i.ensureUniqueRow(checkCtx, row, nil); err != nil {
		return err
	}

	// Advance the table auto-increment value past the inserted row.
	// auto increment
	autoIncIdx := i.t.autoIncIdx
	schemaCols := i.t.schema.Schema
	if autoIncIdx != 0 {
		// Locate the auto-increment column and compare its inserted value.
		autoIncIdx-- // 1-based index
		// ensure next Insert() auto_increment is at least this row + 1
		autoIncVal := i.t.autoIncVal
		if autoIncIdx >= len(schemaCols) {
			return errors.Errorf("auto increment index out of range: %d > %d", autoIncIdx, len(schemaCols)-1)
		}
		autoIncCol := schemaCols[autoIncIdx]
		cmp, err := autoIncCol.Type.Compare(sqlCtx, row[autoIncIdx], autoIncVal)
		if err != nil {
			return errors.Wrap(err, "auto increment type mismatch")
		}

		// Convert larger inserted values into the next auto-increment value.
		if cmp > 0 {
			// Provided value larger than autoIncVal, set autoIncVal to that value
			v, _, err := types.Uint64.Convert(sqlCtx, row[autoIncIdx])
			if err != nil {
				return errors.Wrap(err, "auto increment type mismatch")
			}

			// Require the converted auto-increment value to be an unsigned integer.
			var ok bool
			autoIncVal, ok = v.(uint64)
			if !ok {
				return errors.Wrap(err, "auto increment type mismatch")
			}
			autoIncVal++ // Move onto next autoIncVal
		} else if cmp == 0 {
			autoIncVal++
		}

		// Persist the next auto-increment value in the table root.
		err = i.SetAutoIncrementValue(sqlCtx, autoIncVal)
		if err != nil {
			return err
		}
	}

	// Open the partition transaction for the new row.
	rowKey := MarshalTableRowKey(rowNonce)
	tx, err := pt.BuildTreeTx(i.ctx, false, true)
	if err != nil {
		return err
	}
	rootCursor := tx.GetCursor()

	// Build the inserted row in a detached cursor.
	// detach the root cursor to create a cursor for the new TableRow.
	rowCursor := rootCursor.Detach(false)
	rowCursor.ClearAllRefs()
	_, err = BuildTableRow(cctx, rowCursor, row, i.buildBlobOpts)
	if err != nil {
		return err
	}

	// Attach the inserted row at its nonce key.
	// set the row to the rowKey
	err = tx.SetCursorAtKey(i.ctx, rowKey, rowCursor, false)
	if err != nil {
		return err
	}

	// Record the next row nonce in the dirty table root.
	// increment the row nonce
	i.t.root.RowNonce++
	i.t.bcs.SetBlock(i.t.root, true)
	return nil
}

// Update updates the old row to the new row.
func (i *TableEditor) Update(sqlCtx *sql.Context, oldRow, newRow sql.Row) error {
	// Use the SQL operation context for row access.
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}

	// Provide a SQL context for schema and constraint checks.
	checkCtx := sqlCtx
	if checkCtx == nil {
		checkCtx = sql.NewContext(cctx)
	}

	// Require both row versions to match the table schema.
	schema := i.t.schema.Schema
	if len(oldRow) != len(schema) {
		return sql.ErrInvalidColumnNumber.New(len(schema), len(oldRow))
	}
	if len(newRow) != len(schema) {
		return sql.ErrInvalidColumnNumber.New(len(schema), len(newRow))
	}
	if err := schema.CheckRow(checkCtx, oldRow); err != nil {
		return err
	}
	if err := schema.CheckRow(checkCtx, newRow); err != nil {
		return err
	}

	// Locate the existing row and check the replacement for key conflicts.
	pt, rowKey, err := i.findRowKey(checkCtx, oldRow)
	if err != nil {
		return err
	}
	if err := i.ensureUniqueRow(checkCtx, newRow, rowKey); err != nil {
		return err
	}

	// Build the replacement row in its partition transaction.
	tx, err := pt.BuildTreeTx(cctx, false, true)
	if err != nil {
		return err
	}
	rootCursor := tx.GetCursor()
	rowCursor := rootCursor.Detach(false)
	rowCursor.ClearAllRefs()
	if _, err := BuildTableRow(cctx, rowCursor, newRow, i.buildBlobOpts); err != nil {
		return err
	}

	return tx.SetCursorAtKey(cctx, rowKey, rowCursor, false)
}

// Delete deletes the row given.
func (i *TableEditor) Delete(sqlCtx *sql.Context, row sql.Row) error {
	// Use the SQL operation context for row access.
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}

	// Provide a SQL context for schema and constraint checks.
	checkCtx := sqlCtx
	if checkCtx == nil {
		checkCtx = sql.NewContext(cctx)
	}

	// Require the deleted row to match the table schema.
	if len(row) != len(i.t.schema.Schema) {
		return sql.ErrInvalidColumnNumber.New(len(i.t.schema.Schema), len(row))
	}
	if err := i.t.schema.CheckRow(checkCtx, row); err != nil {
		return err
	}

	// Locate the deleted row in its table partition.
	pt, rowKey, err := i.findRowKey(checkCtx, row)
	if err != nil {
		return err
	}

	// Open the partition transaction that removes the row.
	tx, err := pt.BuildTreeTx(cctx, false, true)
	if err != nil {
		return err
	}

	return tx.Delete(cctx, rowKey)
}

func (i *TableEditor) ensureUniqueRow(sqlCtx *sql.Context, row sql.Row, skipKey []byte) error {
	if len(i.t.schema.PkOrdinals) != 0 {
		found, err := i.hasRowWithEqualValues(sqlCtx, row, i.t.schema.PkOrdinals, skipKey, false)
		if err != nil {
			return err
		}
		if found {
			return sql.ErrPrimaryKeyViolation.New()
		}
	}
	for _, index := range i.t.root.GetIndexes() {
		if !index.GetUnique() {
			continue
		}
		ords, err := i.t.indexColumnOrdinals(index.GetColumns())
		if err != nil {
			return err
		}
		found, err := i.hasRowWithEqualValues(sqlCtx, row, ords, skipKey, true)
		if err != nil {
			return err
		}
		if found {
			return sql.ErrDuplicateEntry.New(index.GetName())
		}
	}
	return nil
}

func (i *TableEditor) hasRowWithEqualValues(
	sqlCtx *sql.Context,
	row sql.Row,
	ordinals []int,
	skipKey []byte,
	skipNull bool,
) (bool, error) {
	// Ignore comparisons with no indexed columns.
	if len(ordinals) == 0 {
		return false, nil
	}

	// Exclude rows with null indexed values when uniqueness permits them.
	if skipNull {
		for _, ord := range ordinals {
			if row[ord] == nil {
				return false, nil
			}
		}
	}

	// Use the SQL operation context for partition reads.
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}

	// Open the table partition iterator for the key comparison.
	partIter, err := i.t.Partitions(sqlCtx)
	if err != nil {
		return false, err
	}
	defer partIter.Close(sqlCtx)

	// Search every table partition for matching indexed values.
	for {
		// Read the next table partition and handle iterator completion.
		part, err := partIter.Next(sqlCtx)
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		// Require a table partition before opening its row transaction.
		pt, ok := part.(*TablePartition)
		if !ok {
			return false, ErrUnexpectedType
		}
		tx, err := pt.BuildTreeTx(cctx, false, false)
		if err != nil {
			return false, err
		}

		// Open the partition row iterator and discard failed transactions.
		rowIter, err := NewTablePartitionRowIter(cctx, tx, i.t.schema.Schema)
		if err != nil {
			tx.Discard()
			return false, err
		}

		// Compare each stored row with the requested indexed values.
		for {
			// Read the next stored row and release resources on failure.
			next, err := rowIter.Next(sqlCtx)
			if err == io.EOF {
				break
			}
			if err != nil {
				rowIter.Close(sqlCtx)
				tx.Discard()
				return false, err
			}

			// Exclude the row being replaced from the uniqueness comparison.
			if skipKey != nil && bytes.Equal(skipKey, rowIter.it.Key()) {
				continue
			}

			// Compare the indexed column values and release resources on failure.
			equals, err := i.rowsEqualOnOrdinals(sqlCtx, row, next, ordinals)
			if err != nil {
				rowIter.Close(sqlCtx)
				tx.Discard()
				return false, err
			}

			// Release the partition resources before reporting a matching row.
			if equals {
				rowIter.Close(sqlCtx)
				tx.Discard()
				return true, nil
			}
		}

		// Release the exhausted partition iterator and transaction.
		rowIter.Close(sqlCtx)
		tx.Discard()
	}
}

func (i *TableEditor) rowsEqualOnOrdinals(sqlCtx *sql.Context, left, right sql.Row, ordinals []int) (bool, error) {
	for _, ord := range ordinals {
		cmp, err := i.t.schema.Schema[ord].Type.Compare(sqlCtx, left[ord], right[ord])
		if err != nil {
			return false, err
		}
		if cmp != 0 {
			return false, nil
		}
	}
	return true, nil
}

func (i *TableEditor) findRowKey(sqlCtx *sql.Context, row sql.Row) (*TablePartition, []byte, error) {
	// Use the SQL operation context when locating the stored row.
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}

	// Open the table partition iterator for the row search.
	partIter, err := i.t.Partitions(sqlCtx)
	if err != nil {
		return nil, nil, err
	}
	defer partIter.Close(sqlCtx)

	// Search each partition for the requested table row.
	for {
		// Read the next table partition and report an exhausted search.
		part, err := partIter.Next(sqlCtx)
		if err == io.EOF {
			return nil, nil, sql.ErrDeleteRowNotFound.New()
		}
		if err != nil {
			return nil, nil, err
		}

		// Require a table partition before opening its row transaction.
		pt, ok := part.(*TablePartition)
		if !ok {
			return nil, nil, ErrUnexpectedType
		}
		tx, err := pt.BuildTreeTx(cctx, false, false)
		if err != nil {
			return nil, nil, err
		}

		// Open the partition row iterator and discard failed transactions.
		rowIter, err := NewTablePartitionRowIter(cctx, tx, i.t.schema.Schema)
		if err != nil {
			tx.Discard()
			return nil, nil, err
		}

		// Compare stored rows until the requested row is found.
		for {
			// Read the next stored row and release resources on failure.
			next, err := rowIter.Next(sqlCtx)
			if err == io.EOF {
				break
			}
			if err != nil {
				rowIter.Close(sqlCtx)
				tx.Discard()
				return nil, nil, err
			}

			// Compare the full row using the table schema.
			equals, err := row.Equals(sqlCtx, next, i.t.schema.Schema)
			if err != nil {
				rowIter.Close(sqlCtx)
				tx.Discard()
				return nil, nil, err
			}

			// Retain the matching row key before releasing partition resources.
			if equals {
				rowKey := bytes.Clone(rowIter.it.Key())
				rowIter.Close(sqlCtx)
				tx.Discard()
				return pt, rowKey, nil
			}
		}

		// Release the exhausted partition iterator and transaction.
		rowIter.Close(sqlCtx)
		tx.Discard()
	}
}

// SetAutoIncrementValue sets a new AUTO_INCREMENT value.
func (i *TableEditor) SetAutoIncrementValue(sqlCtx *sql.Context, val uint64) error {
	// Use the SQL operation context when storing the auto-increment value.
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}

	// Persist the table auto-increment value before updating its cached value.
	err := i.t.root.StoreAutoIncrVal(cctx, i.t.bcs, i.buildBlobOpts, val)
	if err != nil {
		return err
	}

	// Keep the loaded table value consistent with the persisted root.
	i.t.autoIncVal = val
	return nil
}

// AcquireAutoIncrementLock acquires (if necessary) an exclusive lock on generating auto-increment values for the underlying table.
// This is called when @@innodb_autoinc_lock_mode is set to 0 (traditional) or 1 (consecutive), in order to guarantee that insert
// operations get a consecutive range of generated ids. The function returns a callback to release the lock.
func (i *TableEditor) AcquireAutoIncrementLock(ctx *sql.Context) (func(), error) {
	// TODO: determine if it is necessary to implement this here.
	return func() {}, nil
}

// DiscardChanges is called if a statement encounters an error, and all current
// changes since the statement beginning should be discarded.
func (i *TableEditor) DiscardChanges(ctx *sql.Context, errorEncountered error) error {
	// Restore only statements that failed after capturing a table root.
	if errorEncountered == nil || i.statementRoot == nil {
		return nil
	}

	// Use the statement context when restoring the table root.
	cctx := i.ctx
	if ctx != nil && ctx.Context != nil {
		cctx = ctx.Context
	}

	// Consume the saved statement root and reload the table state.
	root := i.statementRoot.CloneVT()
	i.statementRoot = nil
	return i.t.reloadRoot(cctx, root)
}

// StatementComplete is called after the last operation of the statement,
// indicating that it has successfully completed. The mark set in StatementBegin
// may be removed, and a new one should be created on the next StatementBegin.
func (i *TableEditor) StatementComplete(ctx *sql.Context) error {
	i.statementRoot = nil
	return nil
}

// Close finalizes the operation, persisting its result.
func (i *TableEditor) Close(sqlCtx *sql.Context) error {
	// TODO: is it necessary to wait to apply until Close() ?
	return nil
}

// _ is a type assertion
var (
	_ sql.AutoIncrementSetter = (*TableEditor)(nil)
	_ sql.RowInserter         = (*TableEditor)(nil)
	_ sql.RowUpdater          = (*TableEditor)(nil)
	_ sql.RowDeleter          = (*TableEditor)(nil)
)
