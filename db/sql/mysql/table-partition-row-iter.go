package mysql

import (
	"context"
	"io"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// TablePartitionRowIter is a table partition iterator.
type TablePartitionRowIter struct {
	// ctx is the context to use when fetching rows.
	ctx context.Context
	// tree is the kvtx tree transaction.
	tree kvtx.BlockTx
	// it is the tree iterator.
	it kvtx.BlockIterator
	// schema is the table schema.
	schema sql.Schema
}

// NewTablePartitionRowIter constructs a table partition row iterator.
func NewTablePartitionRowIter(
	ctx context.Context,
	tree kvtx.BlockTx,
	schema sql.Schema,
) (*TablePartitionRowIter, error) {
	it := tree.BlockIterate(ctx, nil, false, false)
	return &TablePartitionRowIter{
		ctx:    ctx,
		tree:   tree,
		it:     it,
		schema: schema,
	}, nil
}

// GetRow returns the row at the iterator position, checked against the table
// schema in the SQL context of the query.
func (i *TablePartitionRowIter) GetRow(sctx *sql.Context) (sql.Row, error) {
	// Require a valid partition iterator before decoding its row.
	if err := i.it.Err(); err != nil {
		return nil, err
	}
	if !i.it.Valid() {
		return nil, io.EOF
	}

	// Detach the row cursor so decoded values can be collected after the row
	// read.
	valueCs := i.it.ValueCursor().DetachTransaction()

	// Decode the stored table row block at the detached value cursor.
	tableRow, err := UnmarshalTableRow(i.ctx, valueCs)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"unmarshal table partition row (key %x)", i.it.Key(),
		)
	}

	// Fetch the decoded row values and check them against the table schema.
	sqlRow, err := tableRow.FetchSqlRow(i.ctx, valueCs)
	if err != nil {
		return nil, err
	}
	if err := i.schema.CheckRow(sctx, sqlRow); err != nil {
		return nil, err
	}

	return sqlRow, nil
}

// Next retrieves the next row, or io.EOF after the partition is exhausted.
func (i *TablePartitionRowIter) Next(sctx *sql.Context) (sql.Row, error) {
	// Advance the partition iterator and handle failure or exhaustion.
	if err := i.it.Err(); err != nil {
		return nil, err
	}
	if !i.it.Next() {
		if err := i.it.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	// Decode the table row at the advanced iterator position.
	return i.GetRow(sctx)
}

// Next2 appends the next row to frame, or returns io.EOF when exhausted.
func (i *TablePartitionRowIter) Next2(ctx *sql.Context, frame *sql.RowFrame) error {
	r, err := i.Next(ctx)
	if err != nil {
		return err
	}

	for _, v := range r {
		x, err := sql.ConvertToValue(v)
		if err != nil {
			return err
		}
		frame.Append(x)
	}

	return nil
}

// Close releases the partition iterator and its read transaction.
func (i *TablePartitionRowIter) Close(sctx *sql.Context) error {
	i.it.Close()
	i.tree.Discard()
	return nil
}

// _ is a type assertion
var (
	_ sql.RowIter = (*TablePartitionRowIter)(nil)
)
