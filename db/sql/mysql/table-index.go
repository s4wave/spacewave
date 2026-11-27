package mysql

import (
	"io"
	"strings"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/expression"
	"github.com/pkg/errors"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
)

// tableIndex describes a stored equality index to the SQL planner.
type tableIndex struct {
	// table supplies the indexed column types and stored roots.
	table *Table
	// name identifies the index within the table.
	name string
	// columns contains indexed columns in key order.
	columns []*TableIndexColumn
	// unique indicates that non-null tuples identify at most one row.
	unique bool
	// comment is the index's SQL comment.
	comment string
	// primary selects the row tree instead of a secondary reference tree.
	primary bool
}

// IndexedAccess returns a table that resolves equality lookups through stored keys.
func (t *Table) IndexedAccess(ctx *sql.Context, lookup sql.IndexLookup) sql.IndexedTable {
	return t
}

// GetIndexes returns primary-key and secondary index metadata for planning.
func (t *Table) GetIndexes(ctx *sql.Context) ([]sql.Index, error) {
	indexes := make([]sql.Index, 0, len(t.root.GetIndexes())+1)
	if len(t.schema.PkOrdinals) != 0 {
		cols := make([]*TableIndexColumn, len(t.schema.PkOrdinals))
		for i, ord := range t.schema.PkOrdinals {
			cols[i] = &TableIndexColumn{Name: t.schema.Schema[ord].Name}
		}
		indexes = append(indexes, &tableIndex{
			table:   t,
			name:    "PRIMARY",
			columns: cols,
			unique:  true,
			primary: true,
		})
	}
	for _, index := range t.root.GetIndexes() {
		indexes = append(indexes, &tableIndex{
			table:   t,
			name:    index.GetName(),
			columns: index.GetColumns(),
			unique:  index.GetUnique(),
			comment: index.GetComment(),
		})
	}
	return indexes, nil
}

// PreciseMatch reports whether IndexedAccess can replace predicate filters.
func (t *Table) PreciseMatch() bool {
	return false
}

// LookupPartitions fetches matching rows from the stored index keys.
func (t *Table) LookupPartitions(ctx *sql.Context, lookup sql.IndexLookup) (sql.PartitionIter, error) {
	rows, err := t.lookupRows(ctx, lookup)
	if err != nil {
		return nil, err
	}
	return sql.PartitionsToPartitionIter(&tableLookupPartition{rows: rows}), nil
}

// CreateIndex builds a secondary tree from the existing rows.
func (t *Table) CreateIndex(ctx *sql.Context, def sql.IndexDef) error {
	// Require an unused index name and a supported secondary-index constraint.
	if def.Name == "" {
		return errors.New("index name is empty")
	}
	if def.IsPrimary() {
		return sql.ErrDuplicateKey.New(def.Name)
	}
	if def.IsFullText() || def.IsSpatial() || def.IsVector() {
		return errors.Errorf("unsupported index constraint for %q", def.Name)
	}
	if t.indexByName(def.Name) != nil || strings.EqualFold(def.Name, "PRIMARY") {
		return sql.ErrDuplicateKey.New(def.Name)
	}

	// Resolve the index columns against the table schema.
	cols := make([]*TableIndexColumn, len(def.Columns))
	for i, col := range def.Columns {
		if _, ok := t.columnOrdinal(col.Name); !ok {
			return sql.ErrKeyColumnDoesNotExist.New(col.Name)
		}
		cols[i] = &TableIndexColumn{Name: col.Name, Length: col.Length}
	}

	// Construct the secondary-index metadata from the requested definition.
	index := &TableIndex{
		Name:     def.Name,
		Columns:  cols,
		Unique:   def.IsUnique(),
		Comment:  def.Comment,
		KeyValue: kvtx_block.NewKeyValueStore(0),
	}

	// Build on a detached graph so failure cannot leave a partial index.
	next, err := LoadTable(GetDbContext(ctx), t.name, t.bcs.DetachRecursive(false, true, false))
	if err != nil {
		return err
	}
	next.root.Indexes = append(next.root.Indexes, index)
	next.bcs.SetBlock(next.root, true)
	if err := next.populateIndex(ctx, index); err != nil {
		return err
	}
	return t.replaceRoot(GetDbContext(ctx), next.bcs)
}

// populateIndex builds index entries and detects duplicate unique values.
func (t *Table) populateIndex(ctx *sql.Context, index *TableIndex) error {
	// Open the new secondary tree once for the full table walk.
	tree, err := t.indexTree(ctx, index, true)
	if err != nil {
		return err
	}
	defer tree.Discard()
	for part := range t.root.GetTablePartitions() {
		// Read the old rows with their exact stored primary keys.
		partition, err := t.PartitionAtIndex(part)
		if err != nil {
			return err
		}
		tx, err := partition.BuildTreeTx(GetDbContext(ctx), false, false)
		if err != nil {
			return err
		}
		rows, err := NewTablePartitionRowIter(GetDbContext(ctx), tx, t.schema.Schema)
		if err != nil {
			tx.Discard()
			return err
		}

		// Release the row walk before moving to the next partition.
		err = func() error {
			defer rows.Close(ctx)
			for {
				// Encode the indexed values of each stored row.
				row, primaryKey, err := rows.NextWithKey(ctx)
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return err
				}
				key, unique, err := t.secondaryKey(ctx, index, row, primaryKey)
				if err != nil {
					return err
				}

				// Reject existing non-null unique entries before setting the reference.
				if unique {
					exists, err := tree.Exists(GetDbContext(ctx), key)
					if err != nil {
						return err
					}
					if exists {
						return sql.ErrDuplicateEntry.New(index.GetName())
					}
				}
				if err := tree.Set(GetDbContext(ctx), key, primaryKey); err != nil {
					return err
				}
			}
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

// DropIndex removes a secondary tree and its definition.
func (t *Table) DropIndex(ctx *sql.Context, indexName string) error {
	for i, index := range t.root.GetIndexes() {
		if strings.EqualFold(index.GetName(), indexName) {
			// Retain surviving cursor graphs before changing their positional IDs.
			container := t.bcs.FollowSubBlock(8)
			children, err := container.GetAllRefs(true)
			if err != nil {
				return err
			}
			container.ClearAllRefs()
			t.root.Indexes = append(t.root.Indexes[:i], t.root.Indexes[i+1:]...)

			// Reattach each surviving tree at its new index-set position.
			for id, child := range children {
				if int(id) == i {
					continue
				}
				next := id
				if int(id) > i {
					next--
				}
				if err := child.SetAsSubBlock(next, container); err != nil {
					return err
				}
			}
			t.bcs.SetBlock(t.root, true)
			return nil
		}
	}
	return errors.Errorf("index %q not found", indexName)
}

// RenameIndex renames secondary index metadata.
func (t *Table) RenameIndex(ctx *sql.Context, fromIndexName string, toIndexName string) error {
	// Require an unused secondary-index name.
	if strings.EqualFold(toIndexName, "PRIMARY") || t.indexByName(toIndexName) != nil {
		return sql.ErrDuplicateKey.New(toIndexName)
	}

	// Locate the secondary index being renamed.
	index := t.indexByName(fromIndexName)
	if index == nil {
		return errors.Errorf("index %q not found", fromIndexName)
	}

	// Publish the new index name in the dirty table root.
	index.Name = toIndexName
	t.bcs.SetBlock(t.root, true)
	return nil
}

// indexByName resolves a secondary index using SQL's case-insensitive names.
func (t *Table) indexByName(name string) *TableIndex {
	for _, index := range t.root.GetIndexes() {
		if strings.EqualFold(index.GetName(), name) {
			return index
		}
	}
	return nil
}

// indexColumnOrdinals resolves index column names to their table positions.
func (t *Table) indexColumnOrdinals(columns []*TableIndexColumn) ([]int, error) {
	ords := make([]int, len(columns))
	for i, col := range columns {
		ord, ok := t.columnOrdinal(col.GetName())
		if !ok {
			return nil, sql.ErrKeyColumnDoesNotExist.New(col.GetName())
		}
		ords[i] = ord
	}
	return ords, nil
}

// columnOrdinal finds a column using SQL's case-insensitive names.
func (t *Table) columnOrdinal(name string) (int, bool) {
	for i, col := range t.schema.Schema {
		if strings.EqualFold(col.Name, name) {
			return i, true
		}
	}
	return 0, false
}

// ID returns the index name.
func (idx *tableIndex) ID() string {
	return idx.name
}

// Database returns an empty name because the index belongs to its table alone.
func (idx *tableIndex) Database() string {
	return ""
}

// Table returns the name of the indexed table.
func (idx *tableIndex) Table() string {
	return idx.table.name
}

// Expressions returns the indexed column expressions as strings.
func (idx *tableIndex) Expressions() []string {
	exprs := idx.expressions()
	out := make([]string, len(exprs))
	for i, expr := range exprs {
		out[i] = expr.String()
	}
	return out
}

// IsUnique reports whether the index rejects duplicate keys.
func (idx *tableIndex) IsUnique() bool {
	return idx.unique
}

// IsSpatial reports false because spatial indexes are not supported.
func (idx *tableIndex) IsSpatial() bool {
	return false
}

// IsFullText reports false because full-text indexes are not supported.
func (idx *tableIndex) IsFullText() bool {
	return false
}

// IsVector reports false because vector indexes are not supported.
func (idx *tableIndex) IsVector() bool {
	return false
}

// Comment returns the index comment.
func (idx *tableIndex) Comment() string {
	return idx.comment
}

// IndexType returns BTREE, the type MySQL reports for an ordered index.
func (idx *tableIndex) IndexType() string {
	return "BTREE"
}

// IsGenerated reports false because every index is declared by the user.
func (idx *tableIndex) IsGenerated() bool {
	return false
}

// ColumnExpressionTypes returns each indexed expression with its type.
func (idx *tableIndex) ColumnExpressionTypes(ctx *sql.Context) []sql.ColumnExpressionType {
	exprs := idx.expressions()
	out := make([]sql.ColumnExpressionType, len(exprs))
	for i, expr := range exprs {
		out[i] = sql.ColumnExpressionType{
			Expression: expr.String(),
			Type:       expr.Type(ctx),
		}
	}
	return out
}

// CanSupport accepts ranges that fix the index's leading columns to points,
// optionally followed by a span of the next column when its keys sort in SQL
// order (see orderedType), and the NULL-excluding probe a null-safe join plans
// with before it binds a key. Later columns may select any span: a lookup walks
// the keys under the encoded points within the span, and the engine filters the
// rest because PreciseMatch is false. Other predicates use table scans.
func (idx *tableIndex) CanSupport(ctx *sql.Context, ranges ...sql.Range) bool {
	for _, r := range ranges {
		mysqlRange, ok := r.(sql.MySQLRange)
		if !ok || len(mysqlRange) != len(idx.columns) {
			return false
		}
		if len(leadingPoints(ctx, mysqlRange)) != 0 || idx.spanColumn(ctx, mysqlRange) {
			continue
		}
		for _, column := range mysqlRange {
			if !isNotNullRange(column) {
				return false
			}
		}
	}
	return true
}

// spanColumn reports whether r selects a span of the column after its leading
// points that the index walks as a span of keys.
func (idx *tableIndex) spanColumn(ctx *sql.Context, r sql.MySQLRange) bool {
	n := len(leadingPoints(ctx, r))
	if n == len(r) {
		return false
	}
	ord, ok := idx.table.columnOrdinal(idx.columns[n].GetName())
	return ok && idx.columns[n].GetLength() == 0 && orderedType(idx.table.schema.Schema[ord].Type)
}

// leadingPoints returns the values of the leading columns r fixes to one SQL
// value each, stopping at the first column that selects a span.
func leadingPoints(ctx *sql.Context, r sql.MySQLRange) sql.Row {
	var values sql.Row
	for _, column := range r {
		value, ok := pointValue(ctx, column)
		if !ok {
			break
		}
		values = append(values, value)
	}
	return values
}

// pointValue returns the one SQL value a range column selects, or false when
// the column selects a span.
func pointValue(ctx *sql.Context, column sql.MySQLRangeColumnExpr) (any, bool) {
	// NULL is a point only when both cuts select NULL itself.
	if _, ok := column.LowerBound.(sql.BelowNull); ok {
		_, ok := column.UpperBound.(sql.AboveNull)
		return nil, ok
	}

	// Closed equal lower and upper bounds identify one SQL value.
	lower, lowerOK := column.LowerBound.(sql.Below)
	upper, upperOK := column.UpperBound.(sql.Above)
	if !lowerOK || !upperOK {
		return nil, false
	}
	cmp, err := column.Typ.Compare(ctx, lower.Key, upper.Key)
	if err != nil || cmp != 0 {
		return nil, false
	}
	return lower.Key, true
}

// isNotNullRange reports whether the column selects every non-NULL value.
func isNotNullRange(column sql.MySQLRangeColumnExpr) bool {
	_, lowerOK := column.LowerBound.(sql.AboveNull)
	_, upperOK := column.UpperBound.(sql.AboveAll)
	return lowerOK && upperOK
}

// CanSupportOrderBy reports false: only number columns store keys in SQL order,
// and lookups return rows in no defined order.
func (idx *tableIndex) CanSupportOrderBy(expr sql.Expression) bool {
	return false
}

// CoversColumns reports false because this index reads the underlying table.
func (idx *tableIndex) CoversColumns([]string) bool {
	return false
}

// PrefixLengths returns the key prefix length of each column, or zero for the
// whole value.
func (idx *tableIndex) PrefixLengths() []uint16 {
	out := make([]uint16, len(idx.columns))
	for i, col := range idx.columns {
		if col.GetLength() > 0 && col.GetLength() <= int64(^uint16(0)) {
			out[i] = uint16(col.GetLength()) //nolint:gosec // the explicit positive uint16 bound protects the MySQL prefix field.
		}
	}
	return out
}

// expressions returns planner fields for the index's ordered column list.
func (idx *tableIndex) expressions() []sql.Expression {
	exprs := make([]sql.Expression, len(idx.columns))
	for i, indexCol := range idx.columns {
		ord, _ := idx.table.columnOrdinal(indexCol.GetName())
		col := idx.table.schema.Schema[ord]
		exprs[i] = expression.NewGetFieldWithTable(
			ord,
			0,
			col.Type,
			"",
			idx.table.name,
			col.Name,
			col.Nullable,
		)
	}
	return exprs
}

// _ is a type assertion.
var _ sql.Index = (*tableIndex)(nil)
