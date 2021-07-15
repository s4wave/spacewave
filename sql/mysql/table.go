package mysql

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/pkg/errors"
)

// Table is the block-graph backed data table cursor.
// NOTE: calls are not concurrency-safe.
type Table struct {
	ctx    context.Context
	name   string
	schema sql.Schema
	bcs    *block.Cursor
	root   *TableRoot

	// lookup is the index lookup, nil on default.
	lookup sql.IndexLookup

	// autoIncrIdx is the index of the auto-increment column + 1
	autoIncrIdx int
	// autoIncrVal is the current auto increment value
	autoIncrVal interface{}
}

// LoadTable constructs a new table handle, loading the root block.
func LoadTable(ctx context.Context, name string, bcs *block.Cursor) (*Table, error) {
	// follow the database root
	dbrb, err := bcs.Unmarshal(NewTableRootBlock)
	if err != nil {
		return nil, err
	}
	if dbrb == nil {
		dbrb = NewTableRootBlock()
		bcs.SetBlock(dbrb, true)
	}
	dbr, ok := dbrb.(*TableRoot)
	if !ok {
		return nil, ErrUnexpectedType
	}
	// TODO - is ctx needed here:
	var sctx *sql.Context
	schema, err := dbr.GetTableSchema().ToSqlSchema(sctx)
	if err != nil {
		return nil, err
	}
	// check for auto increment
	var autoIncIdx int
	var autoIncVal interface{}
	for i, colSch := range dbr.GetTableSchema().GetColumns() {
		if colSch.GetAutoIncrement() {
			autoIncIdx = i + 1
			autoIncrType, err := colSch.ParseColumnType()
			if err != nil {
				return nil, errors.Wrapf(err, "table_schema: columns[%d]: type", i)
			}
			autoIncVal, err = dbr.FetchAutoIncrVal(ctx, bcs, autoIncrType)
			if err != nil {
				return nil, errors.Wrapf(err, "table_schema: columns[%d]: auto_incr_val", i)
			}
			break
		}
	}
	return &Table{
		ctx:    ctx,
		name:   name,
		schema: schema,
		bcs:    bcs,
		root:   dbr,

		autoIncrIdx: autoIncIdx,
		autoIncrVal: autoIncVal,
	}, nil
}

// BuildTable constructs a new table, storing it in the block cursor (if set).
//
// if bcs is nil, the returned *Table will also be nil.
func BuildTable(ctx context.Context, bcs *block.Cursor, name string, schema sql.Schema, numPartitions int) (*TableRoot, *Table, error) {
	if numPartitions <= 0 {
		numPartitions = 1
	}
	tr := &TableRoot{
		TableSchema: NewTableSchema(schema),
	}
	tr.TablePartitions = make([]*TablePartitionRoot, numPartitions)
	for i := 0; i < numPartitions; i++ {
		tr.TablePartitions[i] = NewTablePartitionRoot()
	}
	// check for auto increment
	for i, colSch := range tr.GetTableSchema().GetColumns() {
		if colSch.GetAutoIncrement() {
			autoIncrType, err := colSch.ParseColumnType()
			if err != nil {
				return nil, nil, errors.Wrapf(err, "table_schema: columns[%d]", i)
			}
			autoIncrZero := autoIncrType.Zero()
			tr.AutoIncrVal, err = BuildTableColumn(ctx, bcs.FollowSubBlock(4), nil, autoIncrZero)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "table_schema: columns[%d]: build table column", i)
			}
			break
		}
	}

	var err error
	var tbl *Table
	if bcs != nil {
		bcs.SetBlock(tr, true)
		tbl, err = LoadTable(ctx, name, bcs)
	}
	return tr, tbl, err
}

// Name returns the name.
func (t *Table) Name() string {
	return t.name
}

// SetIndexLookup sets the index lookup.
func (t *Table) SetIndexLookup(lookup sql.IndexLookup) {
	t.lookup = lookup
}

// String returns the table in string form.
func (t *Table) String() string {
	// based on String() at go-sql-server/memory/table.go *Table.String
	p := sql.NewTreePrinter()

	kind := ""
	/*
		if len(t.columns) > 0 {
			kind += "Projected "
		}
	*/

	if t.lookup != nil {
		kind += "Indexed "
	}

	if kind != "" {
		kind = ": " + kind
	}

	if len(kind) == 0 {
		return t.name
	}

	_ = p.WriteNode("%s%s", t.name, kind)
	return p.String()
}

// Schema returns the table's SQL schema.
func (t *Table) Schema() sql.Schema {
	return t.schema
}

// PartitionAtIndex returns the partition at an index.
//
// Returns io.EOF if out of range.
func (t *Table) PartitionAtIndex(ix int) (*TablePartition, error) {
	pts := t.root.GetTablePartitions()
	bcs := t.bcs
	if ix >= len(pts) {
		return nil, io.EOF
	}
	pt := pts[ix]
	bcs = bcs.FollowSubBlock(2).FollowSubBlock(uint32(ix))
	var indexLookup sql.IndexLookup // TODO lookup from index
	return NewTablePartition(ix, pt, bcs, t.schema, indexLookup)
}

// Partitions returns an iterator for the table partitions.
func (t *Table) Partitions(ctx *sql.Context) (sql.PartitionIter, error) {
	return NewTablePartitionIter(t), nil
}

// PartitionRows returns a table iterator for the rows in a partition.
func (t *Table) PartitionRows(ctx *sql.Context, part sql.Partition) (sql.RowIter, error) {
	pt, ok := part.(*TablePartition)
	if !ok {
		return nil, ErrUnexpectedType
	}
	return pt.IterateRows(ctx)
}

// SelectPartition selects the partition based on the index (round-robin).
func (t *Table) SelectPartition(nonce uint64) (*TablePartition, int, error) {
	numPts := len(t.root.GetTablePartitions())
	if numPts == 0 {
		return nil, 0, errors.New("no partitions")
	}
	sel := int(nonce % uint64(numPts))
	pt, err := t.PartitionAtIndex(sel)
	return pt, sel, err
}

// PartitionCount returns the number of partitions.
func (t *Table) PartitionCount(*sql.Context) (int64, error) {
	return int64(len(t.root.GetTablePartitions())), nil
}

// Inserter returns a row inserter for the table.
func (t *Table) Inserter(sqlCtx *sql.Context) sql.RowInserter {
	return t.NewTableEditor(sqlCtx)
}

// PeekNextAutoIncrementValue peeks at the next AUTO_INCREMENT value
func (t *Table) PeekNextAutoIncrementValue(*sql.Context) (interface{}, error) {
	return t.autoIncrVal, nil
}

// GetNextAutoIncrementValue gets the next AUTO_INCREMENT value. In the case that a table with an autoincrement
// column is passed in a row with the autoinc column failed, the next auto increment value must
// update its internal state accordingly and use the insert val at runtime.
// Implementations are responsible for updating their state to provide the correct values.
func (t *Table) GetNextAutoIncrementValue(sqlCtx *sql.Context, insertVal interface{}) (interface{}, error) {
	autoIncCol := t.schema[t.autoIncrIdx]
	cmp, err := autoIncCol.Type.Compare(insertVal, t.autoIncrVal)
	if err != nil {
		return nil, err
	}
	if cmp > 0 {
		err = t.AutoIncrementSetter(sqlCtx).SetAutoIncrementValue(sqlCtx, insertVal)
		if err != nil {
			return nil, err
		}
	}
	return t.autoIncrVal, nil
}

// AutoIncrementSetter returns an AutoIncrementSetter.
func (t *Table) AutoIncrementSetter(sqlCtx *sql.Context) sql.AutoIncrementSetter {
	return t.NewTableEditor(sqlCtx)
}

// NewTableEditor constructs a new table editor.
func (t *Table) NewTableEditor(sqlCtx *sql.Context) *TableEditor {
	ctx := t.ctx
	if sqlCtx != nil && sqlCtx.Context != nil {
		ctx = sqlCtx.Context
	}
	return NewTableEditor(ctx, t)
}

// _ is a type assertion
var (
	_ sql.Table              = (*Table)(nil)
	_ sql.PartitionCounter   = (*Table)(nil)
	_ sql.InsertableTable    = (*Table)(nil)
	_ sql.AutoIncrementTable = (*Table)(nil)
	/*
		_ sql.UpdatableTable           = (*Table)(nil)
		_ sql.DeletableTable           = (*Table)(nil)
		_ sql.ReplaceableTable         = (*Table)(nil)
		_ sql.TruncateableTable        = (*Table)(nil)
		_ sql.DriverIndexableTable     = (*Table)(nil)
		_ sql.AlterableTable           = (*Table)(nil)
		_ sql.IndexAlterableTable      = (*Table)(nil)
		_ sql.IndexedTable             = (*Table)(nil)
		_ sql.ForeignKeyAlterableTable = (*Table)(nil)
		_ sql.ForeignKeyTable          = (*Table)(nil)
	*/
)
