package git_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
	"github.com/golang/protobuf/proto"
)

// NewSubmodule constructs a new submodule ref.
func NewSubmodule(name string, ref *block.BlockRef) *Submodule {
	return &Submodule{
		Name:    name,
		RepoRef: ref,
	}
}

// NewSubmoduleBlock builds a new repo ref block.
func NewSubmoduleBlock() block.Block {
	return &Submodule{}
}

// Validate checks the reference.
func (r *Submodule) Validate() error {
	if err := ValidateRefName(r.GetName(), false); err != nil {
		return err
	}
	if r.GetRepoRef().GetEmpty() {
		return ErrReferenceNameEmpty
	}

	if err := r.GetRepoRef().Validate(); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
func (r *Submodule) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
func (r *Submodule) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (r *Submodule) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 2:
		r.RepoRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (r *Submodule) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	if r == nil {
		return nil, nil
	}
	m := make(map[uint32]*block.BlockRef)
	m[2] = r.GetRepoRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (r *Submodule) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 2:
		return NewRepoBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = ((*Submodule)(nil))
	_ block.BlockWithRefs = ((*Submodule)(nil))
	_ sbset.NamedSubBlock = ((*Submodule)(nil))
)
