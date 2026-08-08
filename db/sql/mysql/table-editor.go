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
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}
	checkCtx := sqlCtx
	if checkCtx == nil {
		checkCtx = sql.NewContext(cctx)
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

	if err := i.ensureUniqueRow(checkCtx, row, nil); err != nil {
		return err
	}

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
		cmp, err := autoIncCol.Type.Compare(sqlCtx, row[autoIncIdx], autoIncVal)
		if err != nil {
			return errors.Wrap(err, "auto increment type mismatch")
		}
		if cmp > 0 {
			// Provided value larger than autoIncVal, set autoIncVal to that value
			v, _, err := types.Uint64.Convert(sqlCtx, row[autoIncIdx])
			if err != nil {
				return errors.Wrap(err, "auto increment type mismatch")
			}
			var ok bool
			autoIncVal, ok = v.(uint64)
			if !ok {
				return errors.Wrap(err, "auto increment type mismatch")
			}
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
	err = tx.SetCursorAtKey(i.ctx, rowKey, rowCursor, false)
	if err != nil {
		return err
	}

	// increment the row nonce
	i.t.root.RowNonce++
	i.t.bcs.SetBlock(i.t.root, true)
	return nil
}

// Update updates the old row to the new row.
func (i *TableEditor) Update(sqlCtx *sql.Context, oldRow, newRow sql.Row) error {
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}
	checkCtx := sqlCtx
	if checkCtx == nil {
		checkCtx = sql.NewContext(cctx)
	}
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

	pt, rowKey, err := i.findRowKey(checkCtx, oldRow)
	if err != nil {
		return err
	}
	if err := i.ensureUniqueRow(checkCtx, newRow, rowKey); err != nil {
		return err
	}
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
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}
	checkCtx := sqlCtx
	if checkCtx == nil {
		checkCtx = sql.NewContext(cctx)
	}
	if len(row) != len(i.t.schema.Schema) {
		return sql.ErrInvalidColumnNumber.New(len(i.t.schema.Schema), len(row))
	}
	if err := i.t.schema.CheckRow(checkCtx, row); err != nil {
		return err
	}
	pt, rowKey, err := i.findRowKey(checkCtx, row)
	if err != nil {
		return err
	}
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
	if len(ordinals) == 0 {
		return false, nil
	}
	if skipNull {
		for _, ord := range ordinals {
			if row[ord] == nil {
				return false, nil
			}
		}
	}
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}
	partIter, err := i.t.Partitions(sqlCtx)
	if err != nil {
		return false, err
	}
	defer partIter.Close(sqlCtx)

	for {
		part, err := partIter.Next(sqlCtx)
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		pt, ok := part.(*TablePartition)
		if !ok {
			return false, ErrUnexpectedType
		}
		tx, err := pt.BuildTreeTx(cctx, false, false)
		if err != nil {
			return false, err
		}
		rowIter, err := NewTablePartitionRowIter(cctx, tx, i.t.schema.Schema)
		if err != nil {
			tx.Discard()
			return false, err
		}
		for {
			next, err := rowIter.Next(sqlCtx)
			if err == io.EOF {
				break
			}
			if err != nil {
				rowIter.Close(sqlCtx)
				tx.Discard()
				return false, err
			}
			if skipKey != nil && bytes.Equal(skipKey, rowIter.it.Key()) {
				continue
			}
			equals, err := i.rowsEqualOnOrdinals(sqlCtx, row, next, ordinals)
			if err != nil {
				rowIter.Close(sqlCtx)
				tx.Discard()
				return false, err
			}
			if equals {
				rowIter.Close(sqlCtx)
				tx.Discard()
				return true, nil
			}
		}
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
	cctx := i.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		cctx = sqlCtx.Context
	}
	partIter, err := i.t.Partitions(sqlCtx)
	if err != nil {
		return nil, nil, err
	}
	defer partIter.Close(sqlCtx)

	for {
		part, err := partIter.Next(sqlCtx)
		if err == io.EOF {
			return nil, nil, sql.ErrDeleteRowNotFound.New()
		}
		if err != nil {
			return nil, nil, err
		}
		pt, ok := part.(*TablePartition)
		if !ok {
			return nil, nil, ErrUnexpectedType
		}
		tx, err := pt.BuildTreeTx(cctx, false, false)
		if err != nil {
			return nil, nil, err
		}
		rowIter, err := NewTablePartitionRowIter(cctx, tx, i.t.schema.Schema)
		if err != nil {
			tx.Discard()
			return nil, nil, err
		}
		for {
			next, err := rowIter.Next(sqlCtx)
			if err == io.EOF {
				break
			}
			if err != nil {
				rowIter.Close(sqlCtx)
				tx.Discard()
				return nil, nil, err
			}
			equals, err := row.Equals(sqlCtx, next, i.t.schema.Schema)
			if err != nil {
				rowIter.Close(sqlCtx)
				tx.Discard()
				return nil, nil, err
			}
			if equals {
				rowKey := bytes.Clone(rowIter.it.Key())
				rowIter.Close(sqlCtx)
				tx.Discard()
				return pt, rowKey, nil
			}
		}
		rowIter.Close(sqlCtx)
		tx.Discard()
	}
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
	if errorEncountered == nil || i.statementRoot == nil {
		return nil
	}
	cctx := i.ctx
	if ctx != nil && ctx.Context != nil {
		cctx = ctx.Context
	}
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
