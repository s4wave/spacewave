package mysql

import (
	"context"
	"strconv"

	"github.com/aperturerobotics/hydra/block"
	iavl "github.com/aperturerobotics/hydra/kvtx/block/iavl"
	"github.com/dolthub/go-mysql-server/sql"
)

// TablePartition is a table partition handle.
type TablePartition struct {
	// pt is the partition root
	pt *TablePartitionRoot
	// bcs is the block cursor at the partition sub-block
	bcs *block.Cursor
	// idx is the block index
	idx int
	// schema is the table schema
	schema sql.Schema
	// lookup looks up in an index
	lookup sql.IndexLookup
}

// NewTablePartition constructs a table partition handle.
//
// bcs should be located at the TablePartitionRoot sub-block.
// lookup can be nil
func NewTablePartition(
	idx int,
	t *TablePartitionRoot, bcs *block.Cursor,
	schema sql.Schema,
	lookup sql.IndexLookup,
) (*TablePartition, error) {
	// ensure the partition impl is supported
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return &TablePartition{
		pt:     t,
		bcs:    bcs,
		idx:    idx,
		schema: schema,
		lookup: lookup,
	}, nil
}

// NewTablePartitionRoot constructs a new table partition root object.
func NewTablePartitionRoot() *TablePartitionRoot {
	return &TablePartitionRoot{
		PartitionImpl: PartitionImpl_PartitionImpl_IAVL,
	}
}

// Key returns the partition key.
func (p *TablePartition) Key() []byte {
	return []byte(strconv.Itoa(p.idx))
}

// BuildTreeTx builds the avl tree transaction.
func (p *TablePartition) BuildTreeTx(ctx context.Context, ephemeral bool) (*iavl.Tx, error) {
	// construct iavl tx
	bcs := p.bcs
	if ephemeral {
		bcs = bcs.Detach(true)
	}
	treeBcs := bcs.FollowRef(1, p.pt.GetTreeRef())
	var updateRootCb func(bcs *block.Cursor)
	if !ephemeral {
		updateRootCb = func(bcs *block.Cursor) {
			p.bcs.SetRef(1, bcs, true)
		}
	}
	return iavl.NewTx(ctx, treeBcs, nil, true, updateRootCb)
}

// IterateRows returns a row iterator.
func (p *TablePartition) IterateRows(ctx *sql.Context) (sql.RowIter, error) {
	/* TODO: index lookup
	if p.lookup != nil {
		var err error
		values, err = p.lookup.(sql.DriverIndexLookup).Values(partition)
		if err != nil {
			return nil, err
		}
	}
	*/

	cctx := GetDbContext(ctx)
	tx, err := p.BuildTreeTx(cctx, true)
	if err != nil {
		return nil, err
	}
	return NewTablePartitionRowIter(cctx, tx, p.schema)
}

// _ is a type assertion
var (
	_ sql.Partition = ((*TablePartition)(nil))
)
