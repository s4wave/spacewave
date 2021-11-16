package unixfs_block

import (
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/pkg/errors"
)

// NodeTypeToFSCursorNodeType converts a NodeType to a FSCursorNodeType.
func NodeTypeToFSCursorNodeType(nt NodeType) unixfs.FSCursorNodeType {
	switch nt {
	case NodeType_NodeType_DIRECTORY:
		return unixfs.NewFSCursorNodeType_Dir()
	case NodeType_NodeType_FILE:
		return unixfs.NewFSCursorNodeType_File()
	case NodeType_NodeType_SYMLINK:
		return unixfs.NewFSCursorNodeType_Symlink()
	default:
		return unixfs.NewFSCursorNodeType_Unknown()
	}
}

// FSCursorNodeTypeToNodeType converts a FSCursorNodeType to a NodeType.
func FSCursorNodeTypeToNodeType(nt unixfs.FSCursorNodeType) NodeType {
	if nt == nil {
		return NodeType_NodeType_UNKNOWN
	}
	if nt.GetIsSymlink() {
		return NodeType_NodeType_SYMLINK
	}
	if nt.GetIsDirectory() {
		return NodeType_NodeType_DIRECTORY
	}
	if nt.GetIsFile() {
		return NodeType_NodeType_FILE
	}
	return NodeType_NodeType_UNKNOWN
}

// GetIsDirectory returns if the cursor points to a directory.
func (n NodeType) GetIsDirectory() bool {
	return n == NodeType_NodeType_DIRECTORY
}

// GetIsFile returns if the cursor points to a file.
func (n NodeType) GetIsFile() bool {
	return n == NodeType_NodeType_FILE
}

// GetIsSymlink returns if the cursor points to a symlink.
func (n NodeType) GetIsSymlink() bool {
	return n == NodeType_NodeType_SYMLINK
}

// Validate validates the node type.
func (n NodeType) Validate(allowUnknown bool) error {
	switch n {
	case NodeType_NodeType_DIRECTORY:
	case NodeType_NodeType_FILE:
	case NodeType_NodeType_SYMLINK:
	case NodeType_NodeType_UNKNOWN:
		if !allowUnknown {
			return errors.New("node type cannot be empty")
		}
	default:
		return errors.Errorf("invalid node type: %s", n.String())
	}
	return nil
}
