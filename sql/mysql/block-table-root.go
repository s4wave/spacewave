package mysql

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// NewTableRootBlock constructs a new db root block.
func NewTableRootBlock() block.Block {
	return &TableRoot{}
}

// LoadTableRoot follows the database root cursor.
// may return nil
func LoadTableRoot(cursor *block.Cursor) (*TableRoot, error) {
	ni, err := cursor.Unmarshal(NewTableRootBlock)
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
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (r *TableRoot) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (r *TableRoot) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *TableRoot) ApplySubBlock(id uint32, next block.SubBlock) error {
	// noop
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *TableRoot) GetSubBlocks() map[uint32]block.SubBlock {
	return map[uint32]block.SubBlock{
		2: newTableRootPartitionSetContainer(r, nil),
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
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*TableRoot)(nil))
	_ block.BlockWithSubBlocks = ((*TableRoot)(nil))
)
