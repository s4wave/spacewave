package unixfs_block

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/pkg/errors"
)

// IsNil returns if the object is nil.
func (d *Dirent) IsNil() bool {
	return d == nil
}

// Validate checks the directory entry for validity.
func (d *Dirent) Validate() error {
	if d == nil {
		return errors.New("dirent cannot be nil")
	}
	name := d.GetName()
	if err := ValidateDirentName(name); err != nil {
		return err
	}
	if err := d.GetNode().Validate(false); err != nil {
		return errors.Wrap(err, "dirent node")
	}
	return nil
}

// FollowNode follows the inode sub-block.
// may return nil, bcs if empty
func (d *Dirent) FollowNode(ctx context.Context, bcs *block.Cursor) (*FSNode, *block.Cursor) {
	subRef := bcs.FollowSubBlock(2)
	nod := d.GetNode()
	return nod, subRef
}

// GetIsDirectory returns if the cursor points to a directory.
func (d *Dirent) GetIsDirectory() bool {
	return d.GetNode().GetNodeType().GetIsDirectory()
}

// GetIsFile returns if the cursor points to a file.
func (d *Dirent) GetIsFile() bool {
	return d.GetNode().GetNodeType().GetIsFile()
}

// GetIsSymlink returns if the cursor points to a symlink.
func (d *Dirent) GetIsSymlink() bool {
	return d.GetNode().GetNodeType().GetIsSymlink()
}

// ApplySubBlock applies a sub-block change with a field id.
func (d *Dirent) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 2:
		return block.ApplySubBlock(&d.Node, next)
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (d *Dirent) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	if nod := d.GetNode(); nod != nil {
		m[2] = nod
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (d *Dirent) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return NewFSNodeSubBlockCtor(&d.Node)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.BlockWithSubBlocks = ((*Dirent)(nil))
	_ unixfs.FSCursorDirent    = ((*Dirent)(nil))
)
