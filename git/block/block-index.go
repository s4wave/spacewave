package git_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// NewIndexBlock builds a new index block.
func NewIndexBlock() block.Block {
	return &Index{}
}

// Validate performs cursory validation of the Index.
func (i *Index) Validate() error {
	for idx, ent := range i.GetEntries() {
		if err := ent.Validate(); err != nil {
			return errors.Wrapf(err, "entries[%d]", idx)
		}
	}
	if err := i.GetEndOfIndexEntry().Validate(); err != nil {
		return errors.Wrap(err, "end_of_index_entry")
	}
	if err := i.GetCache().Validate(); err != nil {
		return errors.Wrap(err, "cache")
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (i *Index) MarshalBlock() ([]byte, error) {
	return proto.Marshal(i)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (i *Index) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, i)
}

// ApplySubBlock applies a sub-block change with a field id.
func (i *Index) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 2:
		// ignore
	case 3:
		v, ok := next.(*Tree)
		if !ok {
			return block.ErrUnexpectedType
		}
		i.Cache = v
	case 4:
		v, ok := next.(*ResolveUndo)
		if !ok {
			return block.ErrUnexpectedType
		}
		i.ResolveUndo = v
	case 5:
		v, ok := next.(*EndOfIndexEntry)
		if !ok {
			return block.ErrUnexpectedType
		}
		i.EndOfIndexEntry = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (i *Index) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[2] = NewIndexEntrySet(&i.Entries, nil)
	m[3] = i.GetCache()
	m[4] = i.GetResolveUndo()
	m[5] = i.GetEndOfIndexEntry()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (i *Index) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return func(create bool) block.SubBlock {
			return NewIndexEntrySet(&i.Entries, nil)
		}
	case 3:
		return func(create bool) block.SubBlock {
			v := i.GetCache()
			if create && v == nil {
				v = &Tree{}
				i.Cache = v
			}
			return v
		}
	case 4:
		return func(create bool) block.SubBlock {
			v := i.GetResolveUndo()
			if create && v == nil {
				v = &ResolveUndo{}
				i.ResolveUndo = v
			}
			return v
		}
	case 5:
		return func(create bool) block.SubBlock {
			v := i.GetEndOfIndexEntry()
			if create && v == nil {
				v = &EndOfIndexEntry{}
				i.EndOfIndexEntry = v
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Index)(nil))
	_ block.BlockWithSubBlocks = ((*Index)(nil))
)
