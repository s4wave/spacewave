package git_world

import (
	"github.com/aperturerobotics/hydra/block"
	git_block "github.com/aperturerobotics/hydra/git/block"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/pkg/errors"
)

// NewWorktreeBlock builds a new repo root block.
func NewWorktreeBlock() block.Block {
	return &Worktree{}
}

// UnmarshalWorktree unmarshals a repo from a cursor.
// If empty, returns nil, nil
func UnmarshalWorktree(bcs *block.Cursor) (*Worktree, error) {
	if bcs == nil {
		return nil, nil
	}
	blk, err := bcs.Unmarshal(NewWorktreeBlock)
	if err != nil {
		return nil, err
	}
	if blk == nil {
		return nil, nil
	}
	bv, ok := blk.(*Worktree)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return bv, nil
}

// Validate performs cursory checks on the repo block.
func (r *Worktree) Validate() error {
	if err := r.GetGitIndex().Validate(); err != nil {
		return errors.Wrap(err, "git_index")
	}
	if err := r.GetHeadRefStore().Validate(); err != nil {
		return errors.Wrap(err, "head_ref_store")
	}
	return nil
}

// FollowHeadRefStore follows the head ref store sub-block.
func (r *Worktree) FollowHeadRefStore(bcs *block.Cursor) (*HeadRefStoreCursor, error) {
	subBcs := bcs.FollowSubBlock(2)
	hrs := r.GetHeadRefStore()
	if hrs == nil {
		hrs = &HeadRefStore{}
		r.HeadRefStore = hrs
		subBcs.SetBlock(hrs, true)
	}
	return NewHeadRefStoreCursor(subBcs, hrs), nil
}

// MarshalBlock marshals the block to binary.
func (r *Worktree) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (r *Worktree) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *Worktree) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*git_block.Index)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.GitIndex = v
	case 2:
		v, ok := next.(*HeadRefStore)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.HeadRefStore = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *Worktree) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = r.GetGitIndex()
	m[2] = r.GetHeadRefStore()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *Worktree) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			v := r.GetGitIndex()
			if create && v == nil {
				v = &git_block.Index{}
				r.GitIndex = v
			}
			return v
		}
	case 2:
		return func(create bool) block.SubBlock {
			v := r.GetHeadRefStore()
			if create && v == nil {
				v = &HeadRefStore{}
				r.HeadRefStore = v
			}
			return v
		}
	}
	return nil
}

// Index retrieves the git index.
func (r *Worktree) Index() (*index.Index, error) {
	return r.GetGitIndex().ToGitIndex()
}

// SetIndex sets the index field.
func (r *Worktree) SetIndex(i *index.Index) error {
	ind, err := git_block.NewIndex(i)
	if err != nil {
		return err
	}
	r.GitIndex = ind
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Worktree)(nil))
	_ block.BlockWithSubBlocks = ((*Worktree)(nil))
	_ storer.IndexStorer       = ((*Worktree)(nil))
)
