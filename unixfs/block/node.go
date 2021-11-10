package unixfs_block

import (
	"io/fs"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/file"
	"github.com/aperturerobotics/timestamp"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// NewFSNode constructs a new FSNode.
//
// any non-permissions bits in permissions will be ignored.
// if permissions is empty, will be filled with defaults
// if timestamp is empty, will be filled with a placeholder
func NewFSNode(nt NodeType, permissions fs.FileMode, now *timestamp.Timestamp) *FSNode {
	// set placeholder if nil
	now = FillPlaceholderTimestamp(now)
	if nt == 0 {
		nt = NodeType_NodeType_DIRECTORY
	}
	if permissions == 0 {
		permissions = DefaultPermissions(nt)
	} else {
		permissions = permissions & fs.ModePerm
	}
	return &FSNode{
		NodeType:    nt,
		ModTime:     now,
		Permissions: uint32(permissions),
	}
}

// DefaultPermissions returns the default permissions set for a filetype.
func DefaultPermissions(nt NodeType) fs.FileMode {
	if nt == NodeType_NodeType_DIRECTORY {
		return 0755
	}
	// if nt == NodeType_NodeType_FILE
	return 0644
}

// NewFSNodeBlock constructs a FSNode as a Block.
func NewFSNodeBlock() block.Block {
	return &FSNode{}
}

// FetchCheckFSNode unmarshals a filesystem node and checks its type.
// returns nil, nil if empty
func FetchCheckFSNode(bcs *block.Cursor, nt NodeType) (*FSNode, error) {
	fn, err := UnmarshalFSNode(bcs)
	if err != nil {
		return nil, err
	}
	if fn == nil {
		return nil, nil
	}
	if nt != NodeType_NodeType_UNKNOWN && fn.GetNodeType() != nt {
		return fn, errors.Errorf(
			"expected node type %v but got %v",
			nt.String(),
			fn.GetNodeType().String(),
		)
	}
	if err := fn.Validate(false); err != nil {
		return nil, err
	}
	return fn, nil
}

// UnmarshalFSNode unmarshals a filesystem node from a cursor.
// If empty, returns nil, nil
func UnmarshalFSNode(bcs *block.Cursor) (*FSNode, error) {
	if bcs == nil {
		return nil, nil
	}
	blk, err := bcs.Unmarshal(NewFSNodeBlock)
	if err != nil {
		return nil, err
	}
	if blk == nil {
		return nil, nil
	}
	bv, ok := blk.(*FSNode)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return bv, nil
}

// Validate performs cursory checks of the FS node.
func (n *FSNode) Validate(allowUnknownNodeType bool) error {
	if n.GetPermissions() == 0 {
		return errors.New("permissions cannot be empty")
	}
	if err := n.GetNodeType().Validate(allowUnknownNodeType); err != nil {
		return err
	}
	if perms := n.GetPermissions(); (perms & uint32(fs.ModeType)) != 0 {
		return errors.Errorf("permissions field must not have mode bits set: %d", perms)
	}
	if n.GetModTime().GetTimeUnixMs() == 0 {
		return errors.New("modification time cannot be empty")
	}
	if err := n.GetFile().Validate(); err != nil {
		return err
	}
	var prevName string
	for i, dirent := range n.GetDirectoryEntry() {
		if err := dirent.Validate(); err != nil {
			return errors.Wrapf(err, "directory_entry[%d]", i)
		}
		direntName := dirent.GetName()
		if prevName != "" {
			if direntName == prevName {
				return errors.Errorf("duplicate directory entry: %s", prevName)
			}
			if direntName < prevName {
				// must be sorted
				return errors.Errorf("dirent out of sequence: %s -> %s", prevName, direntName)
			}
		}
		prevName = dirent.Name
	}
	return nil
}

// SetPermissions sets the permissions field from the file mode.
func (n *FSNode) SetPermissions(perms fs.FileMode) {
	if n != nil {
		perms = perms & fs.ModePerm
		n.Permissions = uint32(perms)
	}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (n *FSNode) MarshalBlock() ([]byte, error) {
	return proto.Marshal(n)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (n *FSNode) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, n)
}

// ApplySubBlock applies a sub-block change with a field id.
func (n *FSNode) ApplySubBlock(id uint32, next block.SubBlock) error {
	var ok bool
	switch id {
	case 4:
		n.File, ok = next.(*file.File)
		if !ok {
			return errors.New("sub-block must be a file object")
		}
	case 5:
		var dslice *DirentSlice
		dslice, ok = next.(*DirentSlice)
		if !ok {
			return errors.New("sub-block must be a dirent slice")
		}
		if dslice == nil || dslice.dirents == nil {
			n.DirectoryEntry = nil
		} else {
			n.DirectoryEntry = *dslice.dirents
		}
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (n *FSNode) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[4] = n.GetFile()
	m[5] = NewDirentSlice(&n.DirectoryEntry, nil)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (n *FSNode) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 4:
		return func(create bool) block.SubBlock {
			if n.File == nil && create {
				n.File = &file.File{}
			}
			return n.File
		}
	case 5:
		return func(create bool) block.SubBlock {
			return NewDirentSlice(&n.DirectoryEntry, nil)
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*FSNode)(nil))
	_ block.BlockWithSubBlocks = ((*FSNode)(nil))
)
