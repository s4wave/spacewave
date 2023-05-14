package mysql

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/pkg/errors"
)

// NewTableRootBlock constructs a new db root block.
func NewTableRootBlock() block.Block {
	return &TableRoot{}
}

// LoadTableRoot follows the database root cursor.
// may return nil
func LoadTableRoot(ctx context.Context, cursor *block.Cursor) (*TableRoot, error) {
	ni, err := cursor.Unmarshal(ctx, NewTableRootBlock)
	if err != nil {
		return nil, err
	}
	niv, ok := ni.(*TableRoot)
	if !ok || niv == nil {
		return nil, nil
	}
	if err := niv.Validate(); err != nil {
		return nil, err
	}
	return niv, nil
}

// Validate validates the database root block.
func (r *TableRoot) Validate() error {
	if err := r.GetTableSchema().Validate(); err != nil {
		return errors.Wrap(err, "schema")
	}
	for i, pt := range r.GetTablePartitions() {
		if err := pt.Validate(); err != nil {
			return errors.Wrapf(err, "table_partitions[%d]", i)
		}
	}
	var autoIncrIdx int
	for i, c := range r.GetTableSchema().GetColumns() {
		if c.GetAutoIncrement() {
			autoIncrIdx = i + 1
			break
		}
	}
	autoIncrVal := r.GetAutoIncrVal()
	if autoIncrVal != nil {
		if err := autoIncrVal.Validate(); err != nil {
			return errors.Wrap(err, "auto_incr_val")
		}
	}
	hasAutoIncrCol := !autoIncrVal.IsEmpty()
	if autoIncrIdx == 0 && hasAutoIncrCol {
		return errors.New("expected empty auto_incr_val")
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (r *TableRoot) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (r *TableRoot) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// FetchAutoIncrVal fetches and checks the auto-increment value
//
// bcs should be located at the table root.
func (r *TableRoot) FetchAutoIncrVal(
	ctx context.Context,
	bcs *block.Cursor,
	expectedType sql.Type,
) (interface{}, sql.ConvertInRange, error) {
	autoIncrVal, err := r.GetAutoIncrVal().FetchSqlColumn(ctx, bcs.FollowSubBlock(4))
	if err != nil {
		return nil, false, err
	}
	return expectedType.Convert(autoIncrVal)
}

// StoreAutoIncrVal stores the auto-increment value
//
// bcs should be located at the table root.
func (r *TableRoot) StoreAutoIncrVal(
	ctx context.Context,
	bcs *block.Cursor,
	buildBlobOpts *blob.BuildBlobOpts,
	val interface{},
) error {
	bcs = bcs.FollowSubBlock(4)
	var err error
	r.AutoIncrVal, err = BuildTableColumn(ctx, bcs, buildBlobOpts, val)
	if err != nil {
		return err
	}
	bcs.SetBlock(r.AutoIncrVal, true)
	return nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *TableRoot) ApplySubBlock(id uint32, next block.SubBlock) error {
	var ok bool
	switch id {
	case 4:
		r.AutoIncrVal, ok = next.(*TableColumn)
		if !ok {
			return block.ErrUnexpectedType
		}
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *TableRoot) GetSubBlocks() map[uint32]block.SubBlock {
	return map[uint32]block.SubBlock{
		2: newTableRootPartitionSetContainer(r, nil),
		4: r.GetAutoIncrVal(),
	}
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *TableRoot) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return func(create bool) block.SubBlock {
			return newTableRootPartitionSetContainer(r, nil)
		}
	case 4:
		return func(create bool) block.SubBlock {
			v := r.GetAutoIncrVal()
			if v == nil && create {
				v = &TableColumn{}
				r.AutoIncrVal = v
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*TableRoot)(nil))
	_ block.BlockWithSubBlocks = ((*TableRoot)(nil))
)
