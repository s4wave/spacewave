package unixfs

import (
	"io/fs"
	"os"

	"github.com/pkg/errors"
)

// fsCursorNodeType is a static node type value
type fsCursorNodeType struct {
	// isDir indicates this is a dir
	isDir bool
	// isFile indicates this is a file
	isFile bool
	// isSymlink indicates this is a symlink
	isSymlink bool
}

// NewFSCursorNodeType_Unknown constructs a FSCursorNodeType with no type.
func NewFSCursorNodeType_Unknown() FSCursorNodeType {
	return &fsCursorNodeType{}
}

// NewFSCursorNodeType_File constructs a FSCursorNodeType for a file.
func NewFSCursorNodeType_File() FSCursorNodeType {
	return &fsCursorNodeType{isFile: true}
}

// NewFSCursorNodeType_Dir constructs a FSCursorNodeType for a directory.
func NewFSCursorNodeType_Dir() FSCursorNodeType {
	return &fsCursorNodeType{isDir: true}
}

// NewFSCursorNodeType_Symlink constructs a FSCursorNodeType for a symlink.
func NewFSCursorNodeType_Symlink() FSCursorNodeType {
	return &fsCursorNodeType{isSymlink: true}
}

// NodeTypeToMode converts a fstree node type into a Mode.
func NodeTypeToMode(nodeType FSCursorNodeType, permissions fs.FileMode) fs.FileMode {
	permissions = permissions & fs.ModePerm // filer non-permissions fields
	if nodeType.GetIsSymlink() {
		return permissions | os.ModeSymlink
	}
	if nodeType.GetIsDirectory() {
		return permissions | os.ModeDir
	}
	if nodeType.GetIsFile() {
		// regular file has no mode bits set
		return permissions
	}
	// unknown
	return os.ModeIrregular
}

// ModeToNodeType converts a os.fileMode to a NodeType.
func FileModeToNodeType(mode fs.FileMode) (FSCursorNodeType, error) {
	switch {
	case mode.IsDir():
		return NewFSCursorNodeType_Dir(), nil
	case mode.IsRegular():
		return NewFSCursorNodeType_File(), nil
	case mode&fs.ModeSymlink != 0:
		return NewFSCursorNodeType_Symlink(), nil
	default:
		return nil, errors.Errorf("unsupported mode / node type: %s", mode.String())
	}
}

// GetIsDirectory returns if the node is a directory.
func (f *fsCursorNodeType) GetIsDirectory() bool {
	return f.isDir
}

// GetIsFile returns if the node is a regular file.
func (f *fsCursorNodeType) GetIsFile() bool {
	return f.isFile
}

// GetIsSymlink returns if the node is a symlink.
func (f *fsCursorNodeType) GetIsSymlink() bool {
	return f.isSymlink
}

// _ is a type assertion
var _ FSCursorNodeType = ((*fsCursorNodeType)(nil))
