package unixfs_block

import (
	"context"
	"os"
	"path/filepath"

	"github.com/s4wave/spacewave/db/block"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// CreateFromDisk creates a unixfs_block FSNode from a disk path.
// Unlike CreateFromFS, this captures extended attributes (xattrs) from the
// source filesystem when supported by the platform (macOS, Linux).
func CreateFromDisk(
	ctx context.Context,
	bcs *block.Cursor,
	diskPath string,
	ts *timestamp.Timestamp,
) error {
	iofs := os.DirFS(diskPath)
	rootFsNode := NewFSNode(NodeType_NodeType_DIRECTORY, 0, ts)
	bcs.SetBlock(rootFsNode, true)
	fsTree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		return err
	}
	if err := CopyFSToFSTree(ctx, iofs, fsTree, nil, ts); err != nil {
		return err
	}
	return readDiskXattrsToFSTree(fsTree, diskPath, ".")
}

// readDiskXattrsToFSTree recursively reads xattrs from disk and populates
// the corresponding FSNode.Xattrs fields in the FSTree.
// Platform-specific: implemented on unix, no-op on windows/js.
func readDiskXattrsToFSTree(fsTree *FSTree, diskBasePath string, relPath string) error {
	diskPath := diskBasePath
	if relPath != "." {
		diskPath = filepath.Join(diskBasePath, relPath)
	}
	xattrs, err := readFileXattrs(diskPath)
	if err != nil {
		return err
	}
	node := fsTree.GetFSNode()
	for _, xa := range xattrs {
		node.SetXattr(xa.GetName(), xa.GetValue())
	}

	// Recurse into directory children.
	for _, dirent := range node.GetDirectoryEntry() {
		childName := dirent.GetName()
		child, _, err := fsTree.LookupFollowDirent(childName)
		if err != nil {
			continue
		}
		childRel := childName
		if relPath != "." {
			childRel = filepath.Join(relPath, childName)
		}
		if err := readDiskXattrsToFSTree(child, diskBasePath, childRel); err != nil {
			return err
		}
	}
	return nil
}
