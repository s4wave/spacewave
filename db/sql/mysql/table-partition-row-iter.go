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
	// ctx is the context to use when fetching rows
	ctx context.Context
	// tree is the kvtx tree transaction
	tree kvtx.BlockTx
	// it is the tree iterator
	it kvtx.BlockIterator
	// schema is the table schema
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

// GetRow returns the row at the index.
func (i *TablePartitionRowIter) GetRow() (sql.Row, error) {
	if err := i.it.Err(); err != nil {
		return nil, err
	}
	if !i.it.Valid() {
		return nil, io.EOF
	}
	// check nonce consistency + uint64 marshaling consistency
	rowNonce, err := UnmarshalTableRowKey(i.it.Key())
	if err != nil {
		return nil, err // if len(key) != 8
	}
	// detach to allow Go to garbage-collect the value once we're done.
	valueCs := i.it.ValueCursor().DetachTransaction()
	// follow + fetch table row
	tableRow, err := UnmarshalTableRow(i.ctx, valueCs)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"unmarshal table partition row (nonce %d)", rowNonce,
		)
	}
	sqlRow, err := tableRow.FetchSqlRow(i.ctx, valueCs)
	if err != nil {
		return nil, err
	}
	if err := i.schema.CheckRow(sqlRow); err != nil {
		return nil, err
	}

	return sqlRow, nil
}

// Next retrieves the next row. It will return io.EOF if it's the last row.
// After retrieving the last row, Close will be automatically closed.
func (i *TablePartitionRowIter) Next(sctx *sql.Context) (sql.Row, error) {
	if err := i.it.Err(); err != nil {
		return nil, err
	}
	if !i.it.Next() {
		if err := i.it.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	row, err := i.GetRow()
	if err != nil {
		return nil, err
	}
	/*
		return projectOnRow(i.columns, row), nil
	*/
	return row, nil
}

// Next2 produces the next row, and stores it in the RowFrame provided.
// It will return io.EOF if it's the last row. After retrieving the
// last row, Close will be automatically called.
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

// Close the iterator.
func (i *TablePartitionRowIter) Close(sctx *sql.Context) error {
	i.it.Close()
	// return i.it.Err()
	return nil
}

// _ is a type assertion
var (
	_ sql.RowIter = ((*TablePartitionRowIter)(nil))
)
