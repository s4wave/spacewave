package unixfs_block

import (
	"context"
	"io/fs"
	"path"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/block/file"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// CreateFromFS creates a unixfs_block FSNode from the iofs.
func CreateFromFS(
	ctx context.Context,
	bcs *block.Cursor,
	iofs fs.FS,
	ts *timestamp.Timestamp,
) error {
	rootFsNode := NewFSNode(NodeType_NodeType_DIRECTORY, 0, ts)
	bcs.SetBlock(rootFsNode, true)
	fsTree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err == nil && iofs != nil {
		err = CopyFSToFSTree(ctx, iofs, fsTree, nil, ts)
	}
	return err
}

// CopyFSToFSTree copies the io/fs to the FSTree.
// Copies the data into the in-memory structures quickly.
func CopyFSToFSTree(
	ctx context.Context,
	ifs fs.FS,
	fsTree *FSTree,
	buildBlobOpts *blob.BuildBlobOpts,
	writeTs *timestamp.Timestamp,
) error {
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
			dirents, err := fs.ReadDir(ifs, srcPath)
			if err != nil {
				return err
			}
			for _, ent := range dirents {
				entInfo, err := ent.Info()
				if err != nil {
					return err
				}
				_, entName := path.Split(ent.Name())
				entPath := path.Join(srcPath, entName)
				entType := ent.Type()
				nodeType := FileModeToNodeType(ent.Type())

				// NOTE: io/fs.FS does not support Symlink yet:
				// https://github.com/golang/go/issues/49580
				if nodeType == NodeType_NodeType_UNKNOWN || nodeType == NodeType_NodeType_SYMLINK {
					continue
				}

				// NOTE: "embed" for io/fs strips permissions info & mod time
				entPerm := entInfo.Mode().Perm()
				entNode, err := destNode.Mknod(entName, nodeType, nil, entPerm, writeTs)
				if err != nil {
					return &fs.PathError{}
				}
				pushStack(entPath, entNode, entType.IsDir())
			}

			continue
		}

		fileData, err := fs.ReadFile(ifs, srcPath)
		if err != nil {
			return err
		}

		destFileBcs := destNode.bcs.FollowSubBlock(4)
		destNode.node.File, err = file.BuildFileWithBytes(
			ctx,
			destFileBcs,
			fileData,
			buildBlobOpts,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
