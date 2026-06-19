package mysql

import (
	"io"
	"strings"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/expression"
	"github.com/pkg/errors"
)

type tableIndex struct {
	table   *Table
	name    string
	columns []*TableIndexColumn
	unique  bool
	comment string
	primary bool
}

// IndexedAccess returns a scan-backed indexed table. PreciseMatch is false, so
// the engine keeps predicate filters authoritative.
func (t *Table) IndexedAccess(ctx *sql.Context, lookup sql.IndexLookup) sql.IndexedTable {
	indexed := *t
	indexed.lookup = &lookup
	return &indexed
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

// LookupPartitions returns all partitions for scan-backed indexed access.
func (t *Table) LookupPartitions(ctx *sql.Context, lookup sql.IndexLookup) (sql.PartitionIter, error) {
	indexed := *t
	indexed.lookup = &lookup
	return NewTablePartitionIter(&indexed), nil
}

// CreateIndex records secondary index metadata.
func (t *Table) CreateIndex(ctx *sql.Context, def sql.IndexDef) error {
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
	cols := make([]*TableIndexColumn, len(def.Columns))
	for i, col := range def.Columns {
		if _, ok := t.columnOrdinal(col.Name); !ok {
			return sql.ErrKeyColumnDoesNotExist.New(col.Name)
		}
		cols[i] = &TableIndexColumn{Name: col.Name, Length: col.Length}
	}
	index := &TableIndex{
		Name:    def.Name,
		Columns: cols,
		Unique:  def.IsUnique(),
		Comment: def.Comment,
	}
	if index.GetUnique() {
		editor := t.NewTableEditor(ctx)
		ords, err := t.indexColumnOrdinals(cols)
		if err != nil {
			return err
		}
		partIter, err := t.Partitions(ctx)
		if err != nil {
			return err
		}
		defer partIter.Close(ctx)
		for {
			part, err := partIter.Next(ctx)
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			pt, ok := part.(*TablePartition)
			if !ok {
				return ErrUnexpectedType
			}
			tx, err := pt.BuildTreeTx(GetDbContext(ctx), false, false)
			if err != nil {
				return err
			}
			rowIter, err := NewTablePartitionRowIter(GetDbContext(ctx), tx, t.schema.Schema)
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
				found, err := editor.hasRowWithEqualValues(ctx, row, ords, rowIter.it.Key(), true)
				if err != nil {
					rowIter.Close(ctx)
					tx.Discard()
					return err
				}
				if found {
					rowIter.Close(ctx)
					tx.Discard()
					return sql.ErrDuplicateEntry.New(index.GetName())
				}
			}
			rowIter.Close(ctx)
			tx.Discard()
		}
	}
	t.root.Indexes = append(t.root.Indexes, index)
	t.bcs.SetBlock(t.root, true)
	return nil
}

// DropIndex removes secondary index metadata.
func (t *Table) DropIndex(ctx *sql.Context, indexName string) error {
	for i, index := range t.root.GetIndexes() {
		if strings.EqualFold(index.GetName(), indexName) {
			t.root.Indexes = append(t.root.Indexes[:i], t.root.Indexes[i+1:]...)
			t.bcs.SetBlock(t.root, true)
			return nil
		}
	}
	return errors.Errorf("index %q not found", indexName)
}

// RenameIndex renames secondary index metadata.
func (t *Table) RenameIndex(ctx *sql.Context, fromIndexName string, toIndexName string) error {
	if strings.EqualFold(toIndexName, "PRIMARY") || t.indexByName(toIndexName) != nil {
		return sql.ErrDuplicateKey.New(toIndexName)
	}
	index := t.indexByName(fromIndexName)
	if index == nil {
		return errors.Errorf("index %q not found", fromIndexName)
	}
	index.Name = toIndexName
	t.bcs.SetBlock(t.root, true)
	return nil
}

func (t *Table) indexByName(name string) *TableIndex {
	for _, index := range t.root.GetIndexes() {
		if strings.EqualFold(index.GetName(), name) {
			return index
		}
	}
	return nil
}

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

func (t *Table) columnOrdinal(name string) (int, bool) {
	for i, col := range t.schema.Schema {
		if strings.EqualFold(col.Name, name) {
			return i, true
		}
	}
	return 0, false
}

func (idx *tableIndex) ID() string {
	return idx.name
}

func (idx *tableIndex) Database() string {
	return ""
}

func (idx *tableIndex) Table() string {
	return idx.table.name
}

func (idx *tableIndex) Expressions() []string {
	exprs := idx.expressions()
	out := make([]string, len(exprs))
	for i, expr := range exprs {
		out[i] = expr.String()
	}
	return out
}

func (idx *tableIndex) IsUnique() bool {
	return idx.unique
}

func (idx *tableIndex) IsSpatial() bool {
	return false
}

func (idx *tableIndex) IsFullText() bool {
	return false
}

func (idx *tableIndex) IsVector() bool {
	return false
}

func (idx *tableIndex) Comment() string {
	return idx.comment
}

func (idx *tableIndex) IndexType() string {
	return "BTREE"
}

func (idx *tableIndex) IsGenerated() bool {
	return false
}

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

func (idx *tableIndex) CanSupport(ctx *sql.Context, ranges ...sql.Range) bool {
	return true
}

func (idx *tableIndex) CanSupportOrderBy(expr sql.Expression) bool {
	return false
}

func (idx *tableIndex) PrefixLengths() []uint16 {
	out := make([]uint16, len(idx.columns))
	for i, col := range idx.columns {
		if col.GetLength() > 0 && col.GetLength() <= int64(^uint16(0)) {
			out[i] = uint16(col.GetLength())
		}
	}
	return out
}

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

var _ sql.Index = ((*tableIndex)(nil))
