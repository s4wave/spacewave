package fstree

import (
	"context"
	"errors"
	"syscall"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/block/file"
)

// Mknod creates one or more inodes at the given paths.
func Mknod(root *FSTree, paths [][]string, nodeType NodeType) error {
	var err error
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
			_, err := node.Mkdir(nname)
			if err != nil {
				return err
			}
		} else {
			_, err := node.Mknod(nname, nodeType, nil)
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
) error {
	if len(path) == 0 {
		return errors.New("empty path")
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
	return writer.WriteBytes(uint64(offset), data)
}

// WriteBlob fetches and validates a blob, and then writes it to a offset in a file.
func WriteBlob(
	ctx context.Context,
	root *FSTree,
	path []string,
	offset int64,
	blobSize uint64,
	blobRef *block.BlockRef,
	blobOpts *blob.BuildBlobOpts,
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

	/*
		blobCs := node.GetCursor().FollowRefDetach(blobRef)
		blobBlk, err := blobCs.Unmarshal(blob.NewBlobBlock)
		if err != nil {
			return nil, blobCs, err
		}
		bl := blobBlk.(*blob.Blob)
		totalSize := bl.GetTotalSize()
		if totalSize == 0 {
			return bl, blobCs, errors.New("empty blob")
		}
		if fullValidate {
			if err := bl.ValidateFull(ctx, blobCs); err != nil {
				return bl, blobCs, err
			}
		}
	*/

	fh, err := node.BuildFileHandle(ctx)
	if err != nil {
		return err
	}
	defer fh.Close()

	writer := file.NewWriter(fh, nil, blobOpts)
	return writer.WriteBlob(uint64(offset), blobSize, blobRef)
}

// Remove removes inodes at one or more paths.
// returns if any were removed
func Remove(root *FSTree, paths [][]string) (bool, error) {
	var err error
	var any bool
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
		nodeType := node.GetNode().GetNodeType()
		if nodeType != NodeType_NodeType_DIRECTORY {
			return any, ErrNotDirectory
		}
		nname := path[len(path)-1]
		iany, err := node.Remove([]string{nname})
		if err != nil {
			return any, err
		}
		if iany {
			any = true
		}
	}
	return any, nil
}
