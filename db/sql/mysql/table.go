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
	// ctx supplies the table's default operation context.
	ctx context.Context
	// name is the SQL table name.
	name string
	// schema contains the decoded columns and primary-key positions.
	schema sql.PrimaryKeySchema
	// bcs locates the table root in the block transaction.
	bcs *block.Cursor
	// root contains the stored schema, partitions and indexes.
	root *TableRoot

	// autoIncIdx is the index of the auto-increment column plus one.
	autoIncIdx int
	// autoIncVal is the next auto-increment value.
	autoIncVal uint64
}

// loadRootState loads the schema and counters from a table root.
func (t *Table) loadRootState(ctx context.Context, root *TableRoot) error {
	// Refuse keys written in another encoding: lookups would miss their rows.
	if format := root.GetKeyFormat(); format != tableKeyFormat {
		return errors.Errorf("table key format %d is not %d: recreate the database to rewrite its keys", format, tableKeyFormat)
	}

	// Decode the table schema from its root block.
	schema, err := root.GetTableSchema().ToSqlSchema()
	if err != nil {
		return err
	}

	// Build the SQL primary-key schema from stored column positions.
	pkOrdsVals := root.GetPrimaryKeyOrdinals()
	pkOrds := make([]int, len(pkOrdsVals))
	for i, v := range pkOrdsVals {
		pkOrds[i] = int(v)
	}
	pkSchema := sql.NewPrimaryKeySchema(schema, pkOrds...)

	// Load the table auto-increment column and its stored value.
	var autoIncIdx int
	autoIncVal := uint64(1)
	for i, colSch := range root.GetTableSchema().GetColumns() {
		if !colSch.GetAutoIncrement() {
			continue
		}
		autoIncIdx = i + 1
		autoIncInter, _, err := root.FetchAutoIncrVal(ctx, t.bcs, types.Uint64)
		if err != nil {
			return errors.Wrapf(err, "table_schema: columns[%d]: auto_incr_val", i)
		}
		if autoIncInter != nil {
			var ok bool
			autoIncVal, ok = autoIncInter.(uint64)
			if !ok {
				return errors.New("auto-increment type must be uint64")
			}
			if autoIncVal == 0 {
				autoIncVal = 1
			}
		}
		break
	}

	// Publish the schema and auto-increment state on the loaded table.
	t.schema = pkSchema
	t.root = root
	t.autoIncIdx = autoIncIdx
	t.autoIncVal = autoIncVal
	return nil
}

// replaceRoot adopts a complete detached table graph, including uncommitted rows.
func (t *Table) replaceRoot(ctx context.Context, cursor *block.Cursor) error {
	// Resolve the replacement root and its loaded child graphs before detaching them.
	root, err := block.UnmarshalBlock[TableRoot](ctx, cursor)
	if err != nil {
		return err
	}
	children, err := cursor.GetAllRefs(true)
	if err != nil {
		return err
	}

	// Replace cached sub-block cursors as well as the inline root representation.
	t.bcs.ClearAllRefs()
	t.bcs.SetBlock(root, true)
	for id, child := range children {
		if err := child.SetAsSubBlock(id, t.bcs); err != nil {
			return err
		}
	}
	return t.loadRootState(ctx, root)
}

// LoadTable constructs a new table handle, loading the root block.
func LoadTable(ctx context.Context, name string, bcs *block.Cursor) (*Table, error) {
	// Decode the table root before loading its schema and counters.
	dbr, err := block.UnmarshalBlock[TableRoot](ctx, bcs)
	if err != nil {
		return nil, err
	}
	t := &Table{
		ctx:  ctx,
		name: name,
		bcs:  bcs,
	}
	if err := t.loadRootState(ctx, dbr); err != nil {
		return nil, err
	}
	return t, nil
}

// BuildTable constructs a new table, storing it in the block cursor (if set).
//
// If bcs is nil, the returned *Table will also be nil.
func BuildTable(
	ctx context.Context,
	bcs *block.Cursor,
	name string,
	schema sql.PrimaryKeySchema,
	collationID sql.CollationID,
	comment string,
) (*TableRoot, *Table, error) {
	// Construct the table schema and primary-key column positions.
	tr := &TableRoot{
		KeyFormat:   tableKeyFormat,
		CollationId: uint32(collationID),
		TableSchema: NewTableSchema(schema.Schema),
		Comment:     comment,
	}
	tr.PrimaryKeyOrdinals = make([]int32, len(schema.PkOrdinals))
	for i, v := range schema.PkOrdinals {
		tr.PrimaryKeyOrdinals[i] = int32(v) //nolint:gosec
	}

	// Create the one storage partition that holds every row.
	tr.TablePartitions = []*TablePartitionRoot{NewTablePartitionRoot()}

	// Initialize the table auto-increment column with its type zero value.
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

	// Attach the table root and load its table when a cursor is available.
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

// String returns the table in string form.
func (t *Table) String() string {
	return t.name
}

// Schema returns the table's SQL schema.
func (t *Table) Schema(*sql.Context) sql.Schema {
	return t.schema.Schema
}

// PrimaryKeySchema returns the table schema with its primary key ordinals.
func (t *Table) PrimaryKeySchema(*sql.Context) sql.PrimaryKeySchema {
	return t.schema
}

// PartitionAtIndex returns the partition at an index.
//
// Returns io.EOF if out of range.
func (t *Table) PartitionAtIndex(ix int) (*TablePartition, error) {
	// Require an existing table partition at the requested index.
	pts := t.root.GetTablePartitions()
	bcs := t.bcs
	if ix >= len(pts) {
		return nil, io.EOF
	}

	// Follow the partition block.
	bcs = bcs.FollowSubBlock(2).FollowSubBlock(uint32(ix)) //nolint:gosec
	return NewTablePartition(ix, pts[ix], bcs, t.schema.Schema)
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
//
// Index partitions already contain the rows fetched by their encoded keys.
func (t *Table) PartitionRows(ctx *sql.Context, part sql.Partition) (sql.RowIter, error) {
	// Return rows fetched by an index without opening a table scan.
	if indexed, ok := part.(*tableLookupPartition); ok {
		return sql.RowsToRowIter(indexed.rows...), nil
	}

	// Open an ordinary storage partition for a full table scan.
	pt, ok := part.(*TablePartition)
	if !ok {
		return nil, ErrUnexpectedType
	}
	return pt.IterateRows(ctx)
}

// rowPartition returns the partition that stores every row; BuildTable creates one.
func (t *Table) rowPartition() (*TablePartition, error) {
	return t.PartitionAtIndex(0)
}

// PartitionCount returns the number of partitions.
func (t *Table) PartitionCount(*sql.Context) (int64, error) {
	return int64(len(t.root.GetTablePartitions())), nil
}

// Inserter returns a row inserter for the table.
func (t *Table) Inserter(sqlCtx *sql.Context) sql.RowInserter {
	return t.NewTableEditor(sqlCtx)
}

// Updater returns a row updater for the table.
func (t *Table) Updater(sqlCtx *sql.Context) sql.RowUpdater {
	return t.NewTableEditor(sqlCtx)
}

// Deleter returns a row deleter for the table.
func (t *Table) Deleter(sqlCtx *sql.Context) sql.RowDeleter {
	return t.NewTableEditor(sqlCtx)
}

// Replacer returns a row replacer for the table.
func (t *Table) Replacer(sqlCtx *sql.Context) sql.RowReplacer {
	return t.NewTableEditor(sqlCtx)
}

// PeekNextAutoIncrementValue returns the next AUTO_INCREMENT value without
// reserving it.
func (t *Table) PeekNextAutoIncrementValue(*sql.Context) (uint64, error) {
	return t.autoIncVal, nil
}

// GetNextAutoIncrementValue returns the current counter, raised to insertVal when larger.
func (t *Table) GetNextAutoIncrementValue(sqlCtx *sql.Context, insertVal any) (uint64, error) {
	// Compare the inserted value with the table's next unsigned counter value.
	cmp, err := types.Uint64.Compare(sqlCtx, insertVal, t.autoIncVal)
	if err != nil {
		return 0, err
	}

	// Reserve larger explicit values in the loaded table state.
	if cmp > 0 && insertVal != nil {
		v, _, err := types.Uint64.Convert(sqlCtx, insertVal)
		if err != nil {
			return 0, err
		}
		t.autoIncVal = v.(uint64)
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
	_ sql.Table               = (*Table)(nil)
	_ sql.PrimaryKeyTable     = (*Table)(nil)
	_ sql.PartitionCounter    = (*Table)(nil)
	_ sql.InsertableTable     = (*Table)(nil)
	_ sql.UpdatableTable      = (*Table)(nil)
	_ sql.AutoIncrementTable  = (*Table)(nil)
	_ sql.DeletableTable      = (*Table)(nil)
	_ sql.ReplaceableTable    = (*Table)(nil)
	_ sql.AlterableTable      = (*Table)(nil)
	_ sql.IndexAddressable    = (*Table)(nil)
	_ sql.IndexedTable        = (*Table)(nil)
	_ sql.IndexAlterableTable = (*Table)(nil)
)
