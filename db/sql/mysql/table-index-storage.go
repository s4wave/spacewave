package mysql

import (
	"bytes"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// indexTree opens the secondary index's tree within the table transaction.
func (t *Table) indexTree(ctx *sql.Context, index *TableIndex, write bool) (kvtx.BlockTx, error) {
	for i, candidate := range t.root.GetIndexes() {
		if candidate == index {
			cursor := t.bcs.FollowSubBlock(8).FollowSubBlock(uint32(i)).FollowSubBlock(5) //nolint:gosec
			return index.GetKeyValue().BuildKvTransaction(GetDbContext(ctx), cursor, write)
		}
	}
	return nil, errors.New("index is not attached to table")
}

// secondaryKey encodes index columns, suffixing nonunique and NULL keys by row.
func (t *Table) secondaryKey(ctx *sql.Context, index *TableIndex, row sql.Row, rowKey []byte) ([]byte, bool, error) {
	// Encode indexed values using the table's canonical column representation.
	ordinals, err := t.indexColumnOrdinals(index.GetColumns())
	if err != nil {
		return nil, false, err
	}
	key, hasNull, err := t.columnKey(ctx, row, ordinals, index.GetColumns())
	if err != nil {
		return nil, false, err
	}

	// Keep each duplicate separately while unique non-null values have one key.
	unique := index.GetUnique() && !hasNull
	if !unique {
		key = append(key, rowKey...)
	}
	return key, unique, nil
}

// writeIndexes adds or removes every secondary reference for a stored row.
func (t *Table) writeIndexes(ctx *sql.Context, row sql.Row, rowKey []byte, remove bool) error {
	for _, index := range t.root.GetIndexes() {
		// Compute the index entry before opening its tree.
		key, _, err := t.secondaryKey(ctx, index, row, rowKey)
		if err != nil {
			return err
		}
		tree, err := t.indexTree(ctx, index, true)
		if err != nil {
			return err
		}

		// Mutate the tree through the enclosing block transaction.
		if remove {
			err = tree.Delete(GetDbContext(ctx), key)
		} else {
			err = tree.Set(GetDbContext(ctx), key, rowKey)
		}
		tree.Discard()
		if err != nil {
			return err
		}
	}
	return nil
}

// rowAtKey reads one primary tree key and decodes only that row.
func (t *Table) rowAtKey(ctx *sql.Context, key []byte) (sql.Row, error) {
	// Open the partition that stores every row.
	part, err := t.rowPartition()
	if err != nil {
		return nil, err
	}
	tree, err := part.BuildTreeTx(GetDbContext(ctx), false, false)
	if err != nil {
		return nil, err
	}
	defer tree.Discard()

	// Resolve the single row reference without opening an iterator.
	cursor, err := tree.GetCursorAtKey(GetDbContext(ctx), key)
	if err != nil || cursor == nil {
		return nil, err
	}
	return t.decodeRow(ctx, cursor)
}

// decodeRow decodes the stored row at cursor, enforcing the table schema.
func (t *Table) decodeRow(ctx *sql.Context, cursor *block.Cursor) (sql.Row, error) {
	// Detach the cursor so the decoded row outlives the tree transaction.
	cursor = cursor.DetachTransaction()
	stored, err := UnmarshalTableRow(GetDbContext(ctx), cursor)
	if err != nil {
		return nil, err
	}

	// Enforce the same row/schema contract as ordinary table iteration.
	row, err := stored.FetchSqlRow(GetDbContext(ctx), cursor)
	if err != nil {
		return nil, err
	}
	if err := t.schema.CheckRow(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

// scanRows returns every stored row. A null-safe join probes with a NULL-excluding
// range before it binds a key, and the engine keeps its predicate filters
// because PreciseMatch is false.
func (t *Table) scanRows(ctx *sql.Context) ([]sql.Row, error) {
	// Iterate the one partition that stores every row.
	part, err := t.rowPartition()
	if err != nil {
		return nil, err
	}
	iter, err := part.IterateRows(ctx)
	if err != nil {
		return nil, err
	}
	return sql.RowIterToRows(ctx, iter)
}

// lookupRows resolves the leading equality columns of each range, and a span of
// the next column when its keys sort in SQL order, through the primary or
// secondary tree.
func (t *Table) lookupRows(ctx *sql.Context, lookup sql.IndexLookup) ([]sql.Row, error) {
	// Require ranges supported by the declared index.
	index, ok := lookup.Index.(*tableIndex)
	if !ok {
		return nil, ErrUnexpectedType
	}
	ranges, ok := lookup.Ranges.(sql.MySQLRangeCollection)
	if !ok {
		return nil, ErrUnexpectedType
	}
	ordinals, err := t.indexColumnOrdinals(index.columns)
	if err != nil {
		return nil, err
	}

	// Collect each matching row once across the ranges.
	rows := make([]sql.Row, 0, len(ranges))
	seen := make(map[string]struct{}, len(ranges))
	for _, r := range ranges {
		// Encode the leading points and the next column's span. A range that
		// selects neither scans.
		values := leadingPoints(ctx, r)
		n := len(values)
		var span keySpan
		if index.spanColumn(ctx, r) {
			span = spanOf(ctx, t.schema.Schema[ordinals[n]].Type, r[n])
		} else if n == 0 {
			return t.scanRows(ctx)
		}
		prefix, hasNull, err := t.encodeColumns(ctx, ordinals[:n], values, index.columns[:n])
		if err != nil {
			return nil, err
		}
		single := n == len(index.columns) && (index.primary || index.unique && !hasNull)
		candidates, err := t.indexRows(ctx, index, prefix, span, single)
		if err != nil {
			return nil, err
		}

		// Decode each matching row once even when ranges overlap.
		for _, candidate := range candidates {
			if _, ok := seen[string(candidate.key)]; ok {
				continue
			}

			// Prefix indexes and coerced probes may resolve a wider equality bucket.
			// Check only fetched candidates, never unrelated table rows.
			matches := true
			for i, value := range values {
				cmp, err := r[i].Typ.Compare(ctx, candidate.row[ordinals[i]], value)
				if err != nil {
					return nil, err
				}
				if cmp != 0 {
					matches = false
					break
				}
			}
			if matches {
				seen[string(candidate.key)] = struct{}{}
				rows = append(rows, candidate.row)
			}
		}
	}
	return rows, nil
}

// keyedRow is a decoded row and its primary tree key.
type keyedRow struct {
	// key is the row's primary tree key.
	key []byte
	// row is the decoded row.
	row sql.Row
}

// indexRows reads the rows whose index key begins with prefix, the encoding of
// the index's leading columns, and continues within span. Each encoded component
// is self-delimiting, so the prefix selects exactly the entries with those
// leading values. single marks a complete primary or unique non-null tuple,
// which has at most one entry.
func (t *Table) indexRows(ctx *sql.Context, index *tableIndex, prefix []byte, span keySpan, single bool) ([]keyedRow, error) {
	// The primary tree stores rows under their keys.
	if index.primary {
		if !single {
			return t.rowsInSpan(ctx, prefix, span)
		}
		row, err := t.rowAtKey(ctx, prefix)
		if err != nil || row == nil {
			return nil, err
		}
		return []keyedRow{{key: prefix, row: row}}, nil
	}

	// A secondary tree maps its entries to primary keys.
	tree, err := t.indexTree(ctx, t.indexByName(index.name), false)
	if err != nil {
		return nil, err
	}
	keys, err := secondaryRowKeys(ctx, tree, prefix, span, single)
	tree.Discard()
	if err != nil {
		return nil, err
	}
	rows := make([]keyedRow, 0, len(keys))
	for _, key := range keys {
		row, err := t.rowAtKey(ctx, key)
		if err != nil {
			return nil, err
		}
		if row != nil {
			rows = append(rows, keyedRow{key: key, row: row})
		}
	}
	return rows, nil
}

// rowsInSpan decodes every row whose primary tree key begins with prefix and
// continues within span.
func (t *Table) rowsInSpan(ctx *sql.Context, prefix []byte, span keySpan) ([]keyedRow, error) {
	// Open the partition that stores every row.
	part, err := t.rowPartition()
	if err != nil {
		return nil, err
	}
	tree, err := part.BuildTreeTx(GetDbContext(ctx), false, false)
	if err != nil {
		return nil, err
	}
	defer tree.Discard()

	// Walk only the keys within the span.
	it := tree.BlockIterate(GetDbContext(ctx), prefix, true, false)
	defer it.Close()
	var rows []keyedRow
	err = span.walk(it, prefix, func() error {
		row, err := t.decodeRow(ctx, it.ValueCursor())
		if err != nil {
			return err
		}
		rows = append(rows, keyedRow{key: bytes.Clone(it.Key()), row: row})
		return nil
	})
	return rows, err
}

// secondaryRowKeys resolves one unique key, or every entry under a prefix
// within span.
func secondaryRowKeys(ctx *sql.Context, tree kvtx.BlockTx, prefix []byte, span keySpan, single bool) ([][]byte, error) {
	// A complete unique tuple reads exactly one index entry.
	if single {
		value, found, err := tree.Get(GetDbContext(ctx), prefix)
		if err != nil || !found {
			return nil, err
		}
		return [][]byte{value}, nil
	}

	// Other lookups walk the entries within the span.
	it := tree.Iterate(GetDbContext(ctx), prefix, true, false)
	defer it.Close()
	var keys [][]byte
	err := span.walk(it, prefix, func() error {
		value, err := it.Value()
		if err != nil {
			return err
		}
		keys = append(keys, bytes.Clone(value))
		return nil
	})
	return keys, err
}
