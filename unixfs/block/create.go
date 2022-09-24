package unixfs_block

import (
	"context"
	"io/fs"
	"path"

	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/block/file"
	"github.com/aperturerobotics/timestamp"
)

// IoFS is the minimum set of interfaces for io/fs for CopyFSToFSTree.
type IoFS interface {
	fs.ReadDirFS
	fs.ReadFileFS
}

// CopyFSToFSTree copies the io/fs to the FSTree.
// Copies the data into the in-memory structures quickly.
func CopyFSToFSTree(
	ctx context.Context,
	ifs IoFS,
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
			dirents, err := ifs.ReadDir(srcPath)
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
				entIsDir := ent.IsDir()
				nodeType := NodeType_NodeType_DIRECTORY
				if !entIsDir {
					if ent.Type().IsRegular() {
						nodeType = NodeType_NodeType_FILE
					} else {
						continue
					}
				}
				entNode, err := destNode.Mknod(entName, nodeType, nil, entInfo.Mode().Perm(), writeTs)
				if err != nil {
					return &fs.PathError{}
				}
				pushStack(entPath, entNode, entIsDir)
			}
			continue
		}

		// NOTE: this can be done more efficiently with a fs writer
		// ... conditional on file size: it still might be more efficient this way
		/*
			srcFile, err := ifs.Open(srcPath)
			if err != nil {
				return err
			}
		*/

		fileData, err := ifs.ReadFile(srcPath)
		if err != nil {
			return err
		}

		destFileBcs := destNode.bcs.FollowSubBlock(4)
		destNode.node.File, err = file.BuildFileWithBytes(ctx, destFileBcs, fileData, buildBlobOpts)
		if err != nil {
			return err
		}
	}

	return nil
}
