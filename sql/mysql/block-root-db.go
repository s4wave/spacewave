package mysql

import (
	"github.com/aperturerobotics/hydra/block"
	namedsbset "github.com/aperturerobotics/hydra/block/sbset"
)

// IsNil returns if the object is nil.
func (r *RootDb) IsNil() bool {
	return r == nil
}

// Validate performs cursory checks on the RootDb.
func (r *RootDb) Validate() error {
	if err := r.GetRef().Validate(); err != nil {
		return err
	}
	if len(r.GetName()) == 0 {
		return ErrEmptyDatabaseName
	}
	return nil
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (r *RootDb) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 2:
		r.Ref = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (r *RootDb) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	rr := r.GetRef()
	if rr.GetEmpty() {
		return nil, nil
	}
	m := make(map[uint32]*block.BlockRef)
	m[2] = rr
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (r *RootDb) GetBlockRefCtor(id uint32) block.Ctor {
	return NewDatabaseRootBlock
}

// _ is a type assertion
var (
	_ block.SubBlock           = ((*RootDb)(nil))
	_ block.BlockWithRefs      = ((*RootDb)(nil))
	_ namedsbset.NamedSubBlock = ((*RootDb)(nil))
)
