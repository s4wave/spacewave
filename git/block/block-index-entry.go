package git_block

import (
	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/timestamp"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/pkg/errors"
)

// NewIndexEntry creates a new index entry from a git index entry.
func NewIndexEntry(e *index.Entry) (*IndexEntry, error) {
	if e == nil {
		return nil, nil
	}

	dh, err := NewHash(e.Hash)
	if err != nil {
		return nil, err
	}

	return &IndexEntry{
		DataHash:     dh,
		Name:         e.Name,
		CreatedAt:    timestamp.ToTimestamp(e.CreatedAt),
		ModifiedAt:   timestamp.ToTimestamp(e.ModifiedAt),
		Dev:          e.Dev,
		Inode:        e.Inode,
		FileMode:     uint32(e.Mode),
		Uid:          e.UID,
		Gid:          e.GID,
		Size:         e.Size,
		Stage:        uint32(e.Stage),
		SkipWorktree: e.SkipWorktree,
		IntentToAdd:  e.IntentToAdd,
	}, nil
}

// NewIndexEntryBlock builds a new index entry block.
func NewIndexEntryBlock() block.Block {
	return &IndexEntry{}
}

// IsNil returns if the object is nil.
func (i *IndexEntry) IsNil() bool {
	return i == nil
}

// Validate performs cursory validation of the IndexEntry.
func (i *IndexEntry) Validate() error {
	if err := i.GetDataHash().Validate(); err != nil {
		return errors.Wrap(err, "data_hash")
	}
	if err := i.GetCreatedAt().Validate(true); err != nil {
		return errors.Wrap(err, "created_at")
	}
	if err := i.GetModifiedAt().Validate(true); err != nil {
		return errors.Wrap(err, "modified_at")
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (i *IndexEntry) MarshalBlock() ([]byte, error) {
	return i.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (i *IndexEntry) UnmarshalBlock(data []byte) error {
	return i.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (i *IndexEntry) ApplySubBlock(id uint32, next block.SubBlock) error {
	if id == 1 {
		v, ok := next.(*hash.Hash)
		if !ok {
			return block.ErrUnexpectedType
		}
		i.DataHash = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (i *IndexEntry) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = i.GetDataHash()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (i *IndexEntry) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if id == 1 {
		return func(create bool) block.SubBlock {
			v := i.GetDataHash()
			if create && v == nil {
				v = &hash.Hash{}
				i.DataHash = v
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*IndexEntry)(nil))
	_ block.BlockWithSubBlocks = ((*IndexEntry)(nil))
)
