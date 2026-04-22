package mysql

import (
	"context"
	"io"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// Table is the block-graph backed data table cursor.
// NOTE: calls are not concurrency-safe.
type Table struct {
	ctx    context.Context
	name   string
	schema sql.PrimaryKeySchema
	bcs    *block.Cursor
	root   *TableRoot

	// lookup is the index lookup, nil on default.
	lookup *sql.IndexLookup

	// autoIncIdx is the index of the auto-increment column + 1
	autoIncIdx int
	// autoIncVal is the current auto increment value
	autoIncVal uint64
}

// LoadTable constructs a new table handle, loading the root block.
func LoadTable(ctx context.Context, name string, bcs *block.Cursor) (*Table, error) {
	// follow the database root
	dbr, err := block.UnmarshalBlock[*TableRoot](ctx, bcs, NewTableRootBlock)
	if err != nil {
		return nil, err
	}
	var sctx *sql.Context
	schema, err := dbr.GetTableSchema().ToSqlSchema(sctx)
	if err != nil {
		return nil, err
	}
	pkOrdsVals := dbr.GetPrimaryKeyOrdinals()
	pkOrds := make([]int, len(pkOrdsVals))
	for i, v := range pkOrdsVals {
		pkOrds[i] = int(v)
	}
	pkSchema := sql.NewPrimaryKeySchema(schema, pkOrds...)
	// check for auto increment
	var autoIncIdx int
	var autoIncVal uint64
	for i, colSch := range dbr.GetTableSchema().GetColumns() {
		if colSch.GetAutoIncrement() {
			autoIncIdx = i + 1
			autoIncType := types.Uint64
			var autoIncInter any
			autoIncInter, _, err = dbr.FetchAutoIncrVal(ctx, bcs, autoIncType)
			if err == nil {
				var ok bool
				autoIncVal, ok = autoIncInter.(uint64)
				if !ok {
					err = errors.New("auto-increment type must be uint64")
				}
			}
			if err != nil {
				return nil, errors.Wrapf(err, "table_schema: columns[%d]: auto_incr_val", i)
			}
			break
		}
	}
	return &Table{
		ctx:    ctx,
		name:   name,
		schema: pkSchema,
		bcs:    bcs,
		root:   dbr,

		autoIncIdx: autoIncIdx,
		autoIncVal: autoIncVal,
	}, nil
}

// BuildTable constructs a new table, storing it in the block cursor (if set).
//
// if bcs is nil, the returned *Table will also be nil.
func BuildTable(
	ctx context.Context,
	bcs *block.Cursor,
	name string,
	schema sql.PrimaryKeySchema,
	numPartitions int,
	collationID sql.CollationID,
	comment string,
) (*TableRoot, *Table, error) {
	if numPartitions <= 0 {
		numPartitions = 1
	}
	tr := &TableRoot{
		CollationId: uint32(collationID),
		TableSchema: NewTableSchema(schema.Schema),
		Comment:     comment,
	}
	tr.PrimaryKeyOrdinals = make([]int32, len(schema.PkOrdinals))
	for i, v := range schema.PkOrdinals {
		tr.PrimaryKeyOrdinals[i] = int32(v) //nolint:gosec
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
func (t *Table) SetIndexLookup(lookup *sql.IndexLookup) {
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
func (t *Table) Schema(*sql.Context) sql.Schema {
	return t.schema.Schema
}

// PrimaryKeySchema returns this table's PrimaryKeySchema
func (t *Table) PrimaryKeySchema(*sql.Context) sql.PrimaryKeySchema {
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
	bcs = bcs.FollowSubBlock(2).FollowSubBlock(uint32(ix)) //nolint:gosec
	var indexLookup sql.IndexLookup                        // TODO lookup from index
	// TODO: pkSchema here?
	return NewTablePartition(ix, pt, bcs, t.schema.Schema, indexLookup)
}

// Collation returns the collation type ID.
func (t *Table) Collation() sql.CollationID {
	return sql.CollationID(t.root.GetCollationId()) //nolint:gosec
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
	sel := int(nonce % uint64(numPts)) //nolint:gosec
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
func (t *Table) PeekNextAutoIncrementValue(*sql.Context) (uint64, error) {
	return t.autoIncVal, nil
}

// GetNextAutoIncrementValue gets the next AUTO_INCREMENT value. In the case that a table with an autoincrement
// column is passed in a row with the autoinc column failed, the next auto increment value must
// update its internal state accordingly and use the insert val at runtime.
// Implementations are responsible for updating their state to provide the correct values.
func (t *Table) GetNextAutoIncrementValue(sqlCtx *sql.Context, insertVal any) (uint64, error) {
	/*
		autoIncCol := t.schema.Schema[t.autoIncIdx]
		cmp, err := autoIncCol.Type.Compare(insertVal, t.autoIncVal)
		if err != nil {
			return nil, err
		}
	*/

	cmp, err := types.Uint64.Compare(sqlCtx, insertVal, t.autoIncVal)
	if err != nil {
		return 0, err
	}

	if cmp > 0 && insertVal != nil {
		v, _, err := types.Uint64.Convert(sqlCtx, insertVal)
		if err != nil {
			return 0, err
		}
		t.autoIncVal = v.(uint64)
		/*
			err = t.AutoIncrementSetter(sqlCtx).SetAutoIncrementValue(sqlCtx, insertVal)
			if err != nil {
				return nil, err
			}
		*/
	}
	return t.autoIncVal, nil
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
	_ sql.PrimaryKeyTable    = (*Table)(nil)
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
		_ sql.PrimaryKeyAlterableTable = (*Table)(nil)
		_ sql.IndexAlterableTable      = (*Table)(nil)
		_ sql.IndexedTable             = (*Table)(nil)
		_ sql.ForeignKeyAlterableTable = (*Table)(nil)
		_ sql.ForeignKeyTable          = (*Table)(nil)
	*/
)
