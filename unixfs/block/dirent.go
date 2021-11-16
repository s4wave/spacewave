package unixfs_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/pkg/errors"
)

// Validate checks the directory entry for validity.
func (d *Dirent) Validate() error {
	if d == nil {
		return errors.New("dirent cannot be nil")
	}
	if d.GetName() == "" {
		return errors.New("dirent name cannot be empty")
	}
	if !d.GetNodeRef().GetEmpty() {
		if err := d.GetNodeRef().Validate(); err != nil {
			return errors.Wrap(err, "dirent node_ref")
		}
	}
	if err := d.GetNodeType().Validate(false); err != nil {
		return err
	}
	return nil
}

// FollowNodeRef follows the inode reference.
// returns nil, bcs, nil if not found
func (d *Dirent) FollowNodeRef(bcs *block.Cursor) (*FSNode, *block.Cursor, error) {
	subRef := bcs.FollowRef(2, d.GetNodeRef())
	fn, err := FetchCheckFSNode(subRef, d.GetNodeType())
	return fn, subRef, err
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (d *Dirent) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 2:
		d.NodeRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (d *Dirent) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef)
	m[2] = d.GetNodeRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (d *Dirent) GetBlockRefCtor(id uint32) block.Ctor {
	return NewFSNodeBlock
}

// GetIsDirectory returns if the cursor points to a directory.
func (d *Dirent) GetIsDirectory() bool {
	return d.GetNodeType().GetIsDirectory()
}

// GetIsFile returns if the cursor points to a file.
func (d *Dirent) GetIsFile() bool {
	return d.GetNodeType().GetIsFile()
}

// GetIsSymlink returns if the cursor points to a symlink.
func (d *Dirent) GetIsSymlink() bool {
	return d.GetNodeType().GetIsSymlink()
}

// _ is a type assertion
var (
	_ block.BlockWithRefs   = ((*Dirent)(nil))
	_ unixfs.FSCursorDirent = ((*Dirent)(nil))
)
