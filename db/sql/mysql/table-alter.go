package mysql

import (
	"bytes"
	"io"
	"slices"
	"strings"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/pkg/errors"
)

// AddColumn adds a column and rewrites stored rows to the new schema.
func (t *Table) AddColumn(ctx *sql.Context, column *sql.Column, order *sql.ColumnOrder) error {
	if column == nil {
		return errors.New("column is nil")
	}
	if _, ok := t.columnOrdinal(column.Name); ok {
		return sql.ErrColumnExists.New(column.Name)
	}
	nextColumn := *column
	if nextColumn.Source == "" {
		nextColumn.Source = t.name
	}
	insertAt, err := t.columnOrderIndex(order, len(t.schema.Schema))
	if err != nil {
		return err
	}
	defaultValue, err := columnDefaultValue(ctx, &nextColumn)
	if err != nil {
		return err
	}
	newSchema := slices.Insert(slices.Clone(t.schema.Schema), insertAt, &nextColumn)
	newPK := primaryKeySchemaFor(newSchema)
	if err := t.rewriteRows(ctx, t.schema, newPK, func(row sql.Row) (sql.Row, error) {
		next := make(sql.Row, len(row)+1)
		copy(next, row[:insertAt])
		next[insertAt] = defaultValue
		copy(next[insertAt+1:], row[insertAt:])
		return next, nil
	}); err != nil {
		return err
	}
	t.root.TableSchema = NewTableSchema(newPK.Schema)
	t.setPrimaryKeyOrdinals(newPK.PkOrdinals)
	return t.reloadRoot(GetDbContext(ctx), t.root)
}

// DropColumn drops a column and rewrites stored rows to the new schema.
func (t *Table) DropColumn(ctx *sql.Context, columnName string) error {
	dropAt, ok := t.columnOrdinal(columnName)
	if !ok {
		return sql.ErrTableColumnNotFound.New(t.name, columnName)
	}
	if t.schema.Schema[dropAt].PrimaryKey {
		return errors.Errorf("cannot drop primary key column %q", columnName)
	}
	for _, index := range t.root.GetIndexes() {
		for _, indexCol := range index.GetColumns() {
			if strings.EqualFold(indexCol.GetName(), columnName) {
				return errors.Errorf("cannot drop column %q referenced by index %q", columnName, index.GetName())
			}
		}
	}
	newSchema := slices.Delete(slices.Clone(t.schema.Schema), dropAt, dropAt+1)
	newPK := primaryKeySchemaFor(newSchema)
	if err := t.rewriteRows(ctx, t.schema, newPK, func(row sql.Row) (sql.Row, error) {
		next := make(sql.Row, len(row)-1)
		copy(next, row[:dropAt])
		copy(next[dropAt:], row[dropAt+1:])
		return next, nil
	}); err != nil {
		return err
	}
	t.root.TableSchema = NewTableSchema(newPK.Schema)
	t.setPrimaryKeyOrdinals(newPK.PkOrdinals)
	return t.reloadRoot(GetDbContext(ctx), t.root)
}

// ModifyColumn replaces an existing column definition and optionally moves it.
func (t *Table) ModifyColumn(ctx *sql.Context, columnName string, column *sql.Column, order *sql.ColumnOrder) error {
	oldAt, ok := t.columnOrdinal(columnName)
	if !ok {
		return sql.ErrTableColumnNotFound.New(t.name, columnName)
	}
	if column == nil {
		return errors.New("column is nil")
	}
	nextColumn := *column
	if nextColumn.Source == "" {
		nextColumn.Source = t.name
	}
	withoutOld := slices.Delete(slices.Clone(t.schema.Schema), oldAt, oldAt+1)
	insertAt, err := t.columnOrderIndex(order, len(withoutOld))
	if err != nil {
		return err
	}
	newSchema := slices.Insert(withoutOld, insertAt, &nextColumn)
	newPK := primaryKeySchemaFor(newSchema)
	if err := t.rewriteRows(ctx, t.schema, newPK, func(row sql.Row) (sql.Row, error) {
		value := row[oldAt]
		next := make(sql.Row, 0, len(row))
		next = append(next, row[:oldAt]...)
		next = append(next, row[oldAt+1:]...)
		next = slices.Insert(next, insertAt, value)
		return next, nil
	}); err != nil {
		return err
	}
	t.renameIndexColumn(columnName, nextColumn.Name)
	t.root.TableSchema = NewTableSchema(newPK.Schema)
	t.setPrimaryKeyOrdinals(newPK.PkOrdinals)
	return t.reloadRoot(GetDbContext(ctx), t.root)
}

func (t *Table) rewriteRows(
	ctx *sql.Context,
	oldSchema sql.PrimaryKeySchema,
	newSchema sql.PrimaryKeySchema,
	transform func(sql.Row) (sql.Row, error),
) error {
	cctx := GetDbContext(ctx)
	for ix, ptRoot := range t.root.GetTablePartitions() {
		ptBcs := t.bcs.FollowSubBlock(2).FollowSubBlock(uint32(ix)) //nolint:gosec
		pt, err := NewTablePartition(ix, ptRoot, ptBcs, oldSchema.Schema, sql.IndexLookup{})
		if err != nil {
			return err
		}
		tx, err := pt.BuildTreeTx(cctx, false, true)
		if err != nil {
			return err
		}
		rowIter, err := NewTablePartitionRowIter(cctx, tx, oldSchema.Schema)
		if err != nil {
			tx.Discard()
			return err
		}
		for {
			row, err := rowIter.Next(ctx)
			if err == io.EOF {
				break
			}
			if err != nil {
				rowIter.Close(ctx)
				tx.Discard()
				return err
			}
			rowKey := bytes.Clone(rowIter.it.Key())
			next, err := transform(row)
			if err != nil {
				rowIter.Close(ctx)
				tx.Discard()
				return err
			}
			if err := newSchema.Schema.CheckRow(ctx, next); err != nil {
				rowIter.Close(ctx)
				tx.Discard()
				return err
			}
			rootCursor := tx.GetCursor()
			rowCursor := rootCursor.Detach(false)
			rowCursor.ClearAllRefs()
			if _, err := BuildTableRow(cctx, rowCursor, next, nil); err != nil {
				rowIter.Close(ctx)
				tx.Discard()
				return err
			}
			if err := tx.SetCursorAtKey(cctx, rowKey, rowCursor, false); err != nil {
				rowIter.Close(ctx)
				tx.Discard()
				return err
			}
		}
		rowIter.Close(ctx)
		tx.Discard()
	}
	return nil
}

func (t *Table) columnOrderIndex(order *sql.ColumnOrder, defaultIndex int) (int, error) {
	if order == nil {
		return defaultIndex, nil
	}
	if order.First {
		return 0, nil
	}
	if order.AfterColumn == "" {
		return defaultIndex, nil
	}
	ord, ok := t.columnOrdinal(order.AfterColumn)
	if !ok {
		return 0, sql.ErrTableColumnNotFound.New(t.name, order.AfterColumn)
	}
	return ord + 1, nil
}

func columnDefaultValue(ctx *sql.Context, column *sql.Column) (any, error) {
	if column.Default != nil {
		return column.Default.Eval(ctx, nil)
	}
	if !column.Nullable {
		return nil, errors.Errorf("column %q requires a default", column.Name)
	}
	return nil, nil
}

func primaryKeySchemaFor(schema sql.Schema) sql.PrimaryKeySchema {
	ordinals := make([]int, 0)
	for i, col := range schema {
		if col.PrimaryKey {
			ordinals = append(ordinals, i)
		}
	}
	return sql.NewPrimaryKeySchema(schema, ordinals...)
}

func (t *Table) setPrimaryKeyOrdinals(ordinals []int) {
	t.root.PrimaryKeyOrdinals = make([]int32, len(ordinals))
	for i, ord := range ordinals {
		t.root.PrimaryKeyOrdinals[i] = int32(ord) //nolint:gosec
	}
}

func (t *Table) renameIndexColumn(oldName, newName string) {
	if strings.EqualFold(oldName, newName) {
		return
	}
	for _, index := range t.root.GetIndexes() {
		for _, col := range index.GetColumns() {
			if strings.EqualFold(col.GetName(), oldName) {
				col.Name = newName
			}
		}
	}
}
