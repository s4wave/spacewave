package unixfs_world

import (
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// fileExpect is what a file path must hold when Commit applies the batch.
type fileExpect struct {
	// path is the full path of the file.
	path []string
	// version is the node reference the file must have, or nil when the path
	// must hold nothing.
	version *block.BlockRef
}

// FileConflictError is returned by Commit when a path did not hold what
// ExpectFile required. Nothing was committed.
type FileConflictError struct {
	// Path is the full path of the file.
	Path []string
	// Version is the node reference of the file the path holds, or nil when it
	// holds no file.
	Version *block.BlockRef
}

// Error describes the conflict.
func (e *FileConflictError) Error() string {
	return "file changed by another writer: " + strings.Join(e.Path, "/")
}

// checkFileExpect returns a *FileConflictError unless the tree at root holds
// what e requires. A missing parent, or a parent that is not a directory, holds
// nothing at the path.
func checkFileExpect(root *unixfs_block.FSTree, e fileExpect) error {
	// Find the entry at the path, if any.
	var dirent *unixfs_block.Dirent
	dir, _, err := unixfs_block.LookupFSTreePath(root, e.path[:len(e.path)-1])
	if err == nil {
		dirent, err = dir.Lookup(e.path[len(e.path)-1])
	}
	if err != nil && !errors.Is(err, unixfs_errors.ErrNotExist) && !errors.Is(err, unixfs_errors.ErrNotDirectory) {
		return err
	}

	// Compare it with the expectation.
	var have *block.BlockRef
	if dirent.GetNodeType() == unixfs_block.NodeType_NodeType_FILE {
		have = dirent.GetNodeRef()
	}
	if e.version == nil && dirent == nil {
		return nil
	}
	if e.version != nil && !have.GetEmpty() && have.EqualsRef(e.version) {
		return nil
	}
	return &FileConflictError{Path: slices.Clone(e.path), Version: have.Clone()}
}
