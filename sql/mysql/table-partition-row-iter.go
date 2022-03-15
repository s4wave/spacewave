package mysql

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/kvtx"
	iavl "github.com/aperturerobotics/hydra/kvtx/block/iavl"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/pkg/errors"
)

// TablePartitionRowIter is a table partition iterator.
type TablePartitionRowIter struct {
	// ctx is the context to use when fetching rows
	ctx context.Context
	// tree is the iavl tree tx
	tree *iavl.Tx
	// it is the tree iterator
	it kvtx.BlockIterator
	// schema is the table schema
	schema sql.Schema
}

// NewTablePartitionRowIter constructs a table partition row iterator.
//
// bcs should be located at the root of the iavl tree.
// idx is the starting index for iteration
func NewTablePartitionRowIter(
	ctx context.Context,
	tree *iavl.Tx,
	schema sql.Schema,
) (*TablePartitionRowIter, error) {
	// TODO: Iavl traverse tree iterator
	// this one pre-fetches all keys into RAM in advance.
	it := tree.IterateIavl(nil, false, false)
	return &TablePartitionRowIter{
		ctx:    ctx,
		tree:   tree,
		it:     it,
		schema: schema,
	}, nil
}

// GetRow returns the row at the index.
func (i *TablePartitionRowIter) GetRow() (sql.Row, error) {
	if !i.it.Valid() {
		return nil, io.EOF
	}
	valData := i.it.Value()
	valObj := &TablePartitionRow{}
	if err := valObj.UnmarshalBlock(valData); err != nil {
		return nil, errors.Wrapf(
			err,
			"unmarshal table partition row (length %d)", len(valData),
		)
	}

	/*
		if i.indexValues != nil {
			return i.getFromIndex()
		}
	*/

	// check nonce consistency + uint64 marshaling consistency
	keyNonce, err := UnmarshalTableRowKey(i.it.Key())
	if err != nil {
		return nil, err // if len(key) != 8
	}
	rowNonce := valObj.GetRowNonce()
	if rowNonce != keyNonce {
		return nil, errors.Errorf("key indicated nonce %d but got row nonce %d", keyNonce, rowNonce)
	}

	// follow table row ref
	valCs := i.it.ValueCursor().DetachTransaction()
	tableRowCs := valCs.FollowRef(2, valObj.GetTableRowRef())
	tableRowBlk, err := tableRowCs.Unmarshal(NewTableRowBlock)
	if err != nil {
		return nil, err
	}

	tableRow, ok := tableRowBlk.(*TableRow)
	if !ok {
		return nil, block.ErrUnexpectedType
	}

	sqlRow, err := tableRow.FetchSqlRow(i.ctx, tableRowCs)
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
	_ sql.RowIter  = ((*TablePartitionRowIter)(nil))
	_ sql.RowIter2 = ((*TablePartitionRowIter)(nil))
)
