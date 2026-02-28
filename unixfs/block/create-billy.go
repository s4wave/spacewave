package unixfs_block

import (
	"context"
	"io/fs"
	"path"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/block/file"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	billy "github.com/go-git/go-billy/v6"
)

// CreateFromBillyFS creates a unixfs_block FSNode from the billy FS.
func CreateFromBillyFS(
	ctx context.Context,
	bcs *block.Cursor,
	bfs billy.Filesystem,
	ts *timestamp.Timestamp,
) error {
	rootFsNode := NewFSNode(NodeType_NodeType_DIRECTORY, 0, ts)
	bcs.SetBlock(rootFsNode, true)

	fsTree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err == nil && bfs != nil {
		err = CopyBillyFSToFSTree(ctx, bfs, fsTree, nil, ts)
	}

	return err
}

// CopyBillyFSToFSTree copies the billy filesystem to the FSTree.
// Copies the data into the in-memory structures quickly.
func CopyBillyFSToFSTree(
	ctx context.Context,
	bfs billy.Filesystem,
	fsTree *FSTree,
	buildBlobOpts *blob.BuildBlobOpts,
	writeTs *timestamp.Timestamp,
) error {
	bfsSymlink, bfsSymlinkOk := bfs.(billy.Symlink)

	// stackElem is a element in the fs location stack.
	type stackElem struct {
		// srcPath is the path in the source fs
		srcPath string
		// destNode is the destination node
		destNode *FSTree
		// isDir indicates if this is a directory.
		// otherwise we assume it's a file
		isDir bool
	}

	stack := make([]stackElem, 0, 10)
	pushStack := func(srcPath string, destNode *FSTree, isDir bool) {
		stack = append(stack, stackElem{
			srcPath:  srcPath,
			destNode: destNode,
			isDir:    isDir,
		})
	}

	// assume root node is a directory
	pushStack(".", fsTree, true)

	// recursively traverse filesystem
	for len(stack) != 0 {
		nelem := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		srcPath := nelem.srcPath
		destNode := nelem.destNode

		isDir := nelem.isDir
		if isDir {
			dirents, err := bfs.ReadDir(srcPath)
			if err != nil {
				return err
			}
			for _, entInfo := range dirents {
				_, entName := path.Split(entInfo.Name())
				entPath := path.Join(srcPath, entName)
				entType := entInfo.Type()
				nodeType := FileModeToNodeType(entType)
				if nodeType == NodeType_NodeType_UNKNOWN {
					// Only directory, file, symlink supported.
					continue
				}

				if nodeType == NodeType_NodeType_SYMLINK {
					// Check if the filesystem supports symlinks
					if !bfsSymlinkOk {
						continue
					}

					// Read the symlink
					srcSymlinkPath, err := bfsSymlink.Readlink(entPath)
					if err != nil {
						return &fs.PathError{Op: "readlink", Path: entPath, Err: err}
					}

					// Write the symlink
					_, err = destNode.Symlink(
						false,
						entName,
						NewFSSymlink(SplitFSPath(srcSymlinkPath)),
						writeTs,
					)
					if err != nil {
						return &fs.PathError{Op: "symlink", Path: entPath, Err: err}
					}

					continue
				}

				// NOTE: "embed" for io/fs strips permissions info & mod time
				entFileInfo, err := entInfo.Info()
				if err != nil {
					return &fs.PathError{Op: "stat", Path: entPath, Err: err}
				}
				entPerm := entFileInfo.Mode().Perm()
				entNode, err := destNode.Mknod(entName, nodeType, nil, entPerm, writeTs)
				if err != nil {
					return &fs.PathError{Op: "mknod", Path: entPath, Err: err}
				}

				pushStack(entPath, entNode, nodeType.GetIsDirectory())
			}

			continue
		}

		bfile, err := bfs.Open(srcPath)
		if err != nil {
			return err
		}

		destFileBcs := destNode.bcs.FollowSubBlock(4)
		destNode.node.File, err = file.BuildFileWithReader(
			ctx,
			destFileBcs,
			bfile,
			buildBlobOpts,
		)
		_ = bfile.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
