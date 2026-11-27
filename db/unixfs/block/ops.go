package unixfs_block

import (
	"context"
	"io"
	"io/fs"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// Mknod creates one or more inodes at the given paths.
func Mknod(root *FSTree, paths [][]string, nodeType NodeType, permissions fs.FileMode, ts *timestamp.Timestamp) error {
	ts = FillPlaceholderTimestamp(ts)
	for _, path := range paths {
		if len(path) == 0 {
			continue
		}
		// node := root
		node, _, err := LookupFSTreePath(root, path[:len(path)-1])
		if err != nil {
			return err
		}
		nname := path[len(path)-1]
		if nodeType == NodeType_NodeType_DIRECTORY {
			_, err := node.Mkdir(permissions, ts, nname)
			if err != nil {
				return err
			}
		} else {
			_, err := node.Mknod(nname, nodeType, nil, permissions, ts)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Symlink creates a symbolic link from a location to a path. Returns a cursor
// to the new child node.
func Symlink(root *FSTree, path []string, lnk *FSSymlink, ts *timestamp.Timestamp) (*FSTree, error) {
	// Resolve the parent directory for the symbolic link.
	ts = FillPlaceholderTimestamp(ts)
	node, _, err := LookupFSTreePath(root, path[:len(path)-1])
	if err != nil {
		return nil, err
	}
	nname := path[len(path)-1]
	return node.Symlink(false, nname, lnk, ts)
}

// VisitPaths visits the given list of paths in the fstree.
func VisitPaths(root *FSTree, allowNotExist bool, paths [][]string, cb func(path []string, node *FSTree) error) error {
	for _, path := range paths {
		if len(path) == 0 {
			continue
		}
		node, _, err := LookupFSTreePath(root, path)
		if err != nil {
			if err != unixfs_errors.ErrNotExist || !allowNotExist {
				return err
			}
		}
		if err := cb(path, node); err != nil {
			return err
		}
	}
	return nil
}

// SetPermissions sets the permissions of one or more inodes at the paths.
// The file mode portion of the value is ignored.
func SetPermissions(root *FSTree, paths [][]string, permissions fs.FileMode, ts *timestamp.Timestamp) error {
	var err error
	ts = FillPlaceholderTimestamp(ts)
	return VisitPaths(root, false, paths, func(path []string, node *FSTree) error {
		err = node.SetPermissions(permissions)
		if err != nil {
			return err
		}
		return node.SetModTimestamp(ts)
	})
}

// SetModTimestamp sets the modification time of one or more inodes at the paths.
func SetModTimestamp(root *FSTree, paths [][]string, ts *timestamp.Timestamp) error {
	if ts == nil {
		ts = &timestamp.Timestamp{}
	}
	return VisitPaths(root, false, paths, func(path []string, node *FSTree) error {
		return node.SetModTimestamp(ts)
	})
}

// WriteAt writes data to an offset in an inode (usually a file).
func WriteAt(
	ctx context.Context,
	root *FSTree,
	blobOpts *blob.BuildBlobOpts,
	path []string,
	offset int64,
	writeLen int64,
	dataRdr io.Reader,
	ts *timestamp.Timestamp,
) error {
	// Require a file path before writing its contents.
	if len(path) == 0 {
		return unixfs_errors.ErrEmptyPath
	}

	// Resolve the destination file in the filesystem tree.
	node, _, err := LookupFSTreePath(root, path)
	if err != nil {
		return err
	}

	return node.WriteAt(ctx, blobOpts, offset, writeLen, dataRdr, ts)
}

// WriteBlob fetches and validates a blob, and then writes it to a offset in a file.
// if forceUseBlob is not set, WriteBlob may merge with the previous blob for speed.
func WriteBlob(
	ctx context.Context,
	root *FSTree,
	path []string,
	offset int64,
	blobRef *block.BlockRef,
	fullValidate bool,
	forceUseBlob bool,
	ts *timestamp.Timestamp,
) error {
	// Validate the destination path and blob offset.
	if len(path) == 0 {
		return unixfs_errors.ErrEmptyPath
	}
	if offset < 0 {
		return unixfs_errors.ErrNegativeOffset
	}

	// Resolve the destination file in the filesystem tree.
	node, _, err := LookupFSTreePath(root, path)
	if err != nil {
		return err
	}

	return node.WriteBlob(ctx, uint64(offset), blobRef, fullValidate, forceUseBlob, ts)
}

// CopyOrRename copies or moves an inode from a source path to a destination path.
// Overwrites the destination path.
func CopyOrRename(root *FSTree, srcPath, destPath []string, isMove bool, ts *timestamp.Timestamp) error {
	// Require source and destination paths for the inode transfer.
	if len(srcPath) == 0 || len(destPath) == 0 {
		return unixfs_errors.ErrEmptyPath
	}
	ts = FillPlaceholderTimestamp(ts)

	// source path cannot be a parent of destination path
	// TODO: check the parents recursively to handle symlinks.
	if PathContains(srcPath, destPath) {
		return unixfs_errors.ErrMoveToSelf
	}

	// get node for parent of source
	srcParentNode, _, err := LookupFSTreePath(root, srcPath[:len(srcPath)-1])
	if err != nil {
		return err
	}

	// get node for the src
	srcName := srcPath[len(srcPath)-1]
	srcNode, _, err := srcParentNode.LookupFollowDirent(srcName)
	if err == nil && srcNode == nil {
		err = unixfs_errors.ErrNotExist
	}
	if err != nil {
		return err
	}

	// get node for parent of destination
	destName := destPath[len(destPath)-1]
	destParentNode, _, err := LookupFSTreePath(root, destPath[:len(destPath)-1])
	if err != nil {
		return err
	}

	// Attach a detached copy of the source inode to the destination directory.
	if err := srcNode.copyTo(destParentNode, destName); err != nil {
		return err
	}

	// remove old pos if move
	if isMove {
		_, err = srcParentNode.Remove([]string{srcName}, ts)
		if err != nil {
			return err
		}
	}

	return nil
}

// TruncateFile changes the size of a file.
func TruncateFile(
	ctx context.Context,
	root *FSTree,
	path []string,
	nsize int64,
	ts *timestamp.Timestamp,
) error {
	// Require a file path before changing its size.
	if len(path) == 0 {
		return unixfs_errors.ErrEmptyPath
	}
	ts = FillPlaceholderTimestamp(ts)
	if nsize < 0 {
		nsize = 0
	}

	// Resolve the file whose size will change.
	node, _, err := LookupFSTreePath(root, path)
	if err != nil {
		return err
	}

	return node.TruncateFile(ctx, nsize, ts)
}

// Remove removes inodes at one or more paths and reports whether any were
// removed.
func Remove(root *FSTree, paths [][]string, ts *timestamp.Timestamp) (bool, error) {
	var any bool
	ts = FillPlaceholderTimestamp(ts)
	for _, path := range paths {
		if len(path) == 0 {
			continue
		}

		node, _, err := LookupFSTreePath(root, path[:len(path)-1])
		if err != nil {
			return false, err
		}

		nodeType := node.GetNodeType()
		if nodeType != NodeType_NodeType_DIRECTORY {
			return any, unixfs_errors.ErrNotDirectory
		}

		nname := path[len(path)-1]
		iany, err := node.Remove([]string{nname}, ts)
		if err != nil {
			return any, err
		}
		if iany {
			any = true
		}
	}
	return any, nil
}

// LookupFSTreePath repeatedly calls LookupFollowDirent to traverse to a path.
// Returns the parent FSNode and ErrNotExist if path does not exist.
//
// Note: if ErrNotExist is returned, LookupFSTreePath also returns the node
// where the lookup returning ErrNotExist occurred.
//
// Returns the path to the returned FSTree node. The path will be a subset
// of the full path passed as an argument. The same slice memory will be used
// for the returned path: changing the values in that slice will change the path
// argument slice as well. Do not change this slice without copying it first.
func LookupFSTreePath(node *FSTree, path []string) (*FSTree, []string, error) {
	for i, dir := range path {
		nextNode, _, err := node.LookupFollowDirent(dir)
		if err == nil && nextNode == nil {
			err = unixfs_errors.ErrNotExist
		}
		if err != nil {
			return node, path[:i], err
		}
		node = nextNode
	}
	return node, path, nil
}
