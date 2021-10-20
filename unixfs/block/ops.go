package unixfs_block

import (
	"context"
	"errors"
	"syscall"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/block/file"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/timestamp"
)

// Mknod creates one or more inodes at the given paths.
func Mknod(root *FSTree, paths [][]string, nodeType NodeType, permissions uint32, ts *timestamp.Timestamp) error {
	var err error
	ts = FillPlaceholderTimestamp(ts)
	for _, path := range paths {
		if len(path) == 0 {
			continue
		}
		node := root
		for _, dir := range path[:len(path)-1] {
			node, _, err = node.LookupFollowDirent(dir)
			if err != nil {
				return err
			}
			if node == nil {
				return syscall.ENOENT
			}
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

// Write writes data to an offset in an inode (usually a file).
func Write(
	ctx context.Context,
	root *FSTree,
	blobOpts *blob.BuildBlobOpts,
	path []string,
	offset int64,
	data []byte,
	ts *timestamp.Timestamp,
) error {
	if len(path) == 0 {
		return errors.New("empty path")
	}
	if ts == nil {

	}

	var err error
	node := root
	for _, dir := range path {
		node, _, err = node.LookupFollowDirent(dir)
		if err != nil {
			return err
		}
		if node == nil {
			return syscall.ENOENT
		}
	}

	fh, err := node.BuildFileHandle(ctx)
	if err != nil {
		return err
	}
	defer fh.Close()

	writer := file.NewWriter(fh, nil, blobOpts)
	err = writer.WriteBytes(uint64(offset), data)
	if err != nil {
		return err
	}

	// set placeholder if nil
	ts = FillPlaceholderTimestamp(ts)
	node.node.ModTime = ts

	node.bcs.SetBlock(node.node, true)
	return nil
}

// WriteBlob fetches and validates a blob, and then writes it to a offset in a file.
func WriteBlob(
	ctx context.Context,
	root *FSTree,
	path []string,
	offset int64,
	blobRef *block.BlockRef,
	fullValidate bool,
	ts *timestamp.Timestamp,
) error {
	if len(path) == 0 {
		return errors.New("empty path")
	}
	if offset < 0 {
		return errors.New("negative offset not supported")
	}

	var err error
	node := root
	for _, dir := range path {
		node, _, err = node.LookupFollowDirent(dir)
		if err != nil {
			return err
		}
		if node == nil {
			return syscall.ENOENT
		}
	}

	blobCs := node.GetCursor().DetachTransaction()
	blobCs.SetRefAtCursor(blobRef, true)
	blk, err := blobCs.Unmarshal(blob.NewBlobBlock)
	if err != nil {
		return err
	}
	bl, ok := blk.(*blob.Blob)
	if !ok {
		return block.ErrUnexpectedType
	}

	totalSize := bl.GetTotalSize()
	if totalSize == 0 {
		return errors.New("empty blob")
	}
	if fullValidate {
		if err := bl.ValidateFull(ctx, blobCs); err != nil {
			return err
		}
	}

	fh, err := node.BuildFileHandle(ctx)
	if err != nil {
		return err
	}
	defer fh.Close()

	writer := file.NewWriter(fh, nil, nil)
	err = writer.WriteBlob(uint64(offset), totalSize, blobRef)
	if err != nil {
		return err
	}

	// update timestamp
	// set placeholder if nil
	ts = FillPlaceholderTimestamp(ts)
	node.node.ModTime = ts

	node.bcs.SetBlock(node.node, true)
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
	if len(path) == 0 {
		return errors.New("empty path")
	}
	ts = FillPlaceholderTimestamp(ts)
	if nsize < 0 {
		nsize = 0
	}

	var err error
	node := root
	for _, dir := range path {
		node, _, err = node.LookupFollowDirent(dir)
		if err != nil {
			return err
		}
		if node == nil {
			return syscall.ENOENT
		}
	}

	fh, err := node.BuildFileHandle(ctx)
	if err != nil {
		return err
	}
	defer fh.Close()

	if fh.Size() == uint64(nsize) {
		// no-op
		return nil
	}

	writer := file.NewWriter(fh, nil, nil)
	err = writer.Truncate(uint64(nsize))
	if err != nil {
		return err
	}

	// update timestamp
	node.node.ModTime = ts

	node.bcs.SetBlock(node.node, true)
	return nil
}

// Remove removes inodes at one or more paths.
// returns if any were removed
func Remove(root *FSTree, paths [][]string, ts *timestamp.Timestamp) (bool, error) {
	var err error
	var any bool
	ts = FillPlaceholderTimestamp(ts)
	for _, path := range paths {
		if len(path) == 0 {
			continue
		}
		node := root
		for _, dir := range path[:len(path)-1] {
			node, _, err = node.LookupFollowDirent(dir)
			if err != nil {
				return any, err
			}
			if node == nil {
				return any, syscall.ENOENT
			}
		}
		nodeType := node.GetFSNode().GetNodeType()
		if nodeType != NodeType_NodeType_DIRECTORY {
			return any, unixfs_errors.ErrNotDirectory
		}
		nname := path[len(path)-1]
		iany, err := node.Remove([]string{nname})
		if err != nil {
			return any, err
		}
		if iany {
			any = true
			node.node.ModTime = ts
		}
	}
	return any, nil
}
