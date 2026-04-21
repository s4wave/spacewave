package unixfs_block

import (
	"slices"

	"github.com/s4wave/spacewave/db/block"
)

// NewFSSymlink constructs a new symlink object.
func NewFSSymlink(tgtPath *FSPath) *FSSymlink {
	return &FSSymlink{TargetPath: tgtPath}
}

// IsNil returns if the object is nil.
func (s *FSSymlink) IsNil() bool {
	return s == nil
}

// Validate checks the symlink data for validity.
// Symlink targets allow "." and ".." path components (relative symlinks).
func (s *FSSymlink) Validate() error {
	tp := s.GetTargetPath()
	if slices.Contains(tp.GetNodes(), "") {
		return ErrDirectoryNameEmpty
	}
	return nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (s *FSSymlink) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		tgt, ok := next.(*FSPath)
		if !ok {
			return block.ErrUnexpectedType
		}
		s.TargetPath = tgt
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (s *FSSymlink) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = s.GetTargetPath()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (s *FSSymlink) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			if s.TargetPath == nil && create {
				s.TargetPath = &FSPath{}
			}
			return s.TargetPath
		}
	}
	return nil
}

// _ is a type assertion
var _ block.BlockWithSubBlocks = ((*FSSymlink)(nil))
