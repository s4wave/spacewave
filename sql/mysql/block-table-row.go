package mysql

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/dolthub/go-mysql-server/sql"
)

// NewTableRowBlock constructs a new db root block.
func NewTableRowBlock() block.Block {
	return &TableRow{}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (r *TableRow) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (r *TableRow) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *TableRow) ApplySubBlock(id uint32, next block.SubBlock) error {
	// noop
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *TableRow) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = newTableRowColumnSetContainer(r, nil)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *TableRow) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			return newTableRowColumnSetContainer(r, nil)
		}
	}
	return nil
}

// FetchSqlRow fetches columns into a sql.Row structure.
// This converts the values using proto.Any into Go types.
// The resulting sql.Row should be checked against a schema.
func (i *TableRow) FetchSqlRow(ctx context.Context, bcs *block.Cursor) (sql.Row, error) {
	colSet := newTableRowColumnSetContainer(i, bcs)
	rowCols := i.GetColumns()
	cols := make(sql.Row, len(rowCols))
	for i, col := range rowCols {
		_, colcs := colSet.Get(i)
		r, err := col.FetchSqlColumn(ctx, colcs)
		if err != nil {
			return nil, err
		}
		cols[i] = r
	}
	return cols, nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*TableRow)(nil))
	_ block.BlockWithSubBlocks = ((*TableRow)(nil))
)
