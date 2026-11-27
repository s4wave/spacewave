// Package layer builds UnixFS layers from file entries and lays layers onto a
// root by reference.
//
// Every node a layer writes has the same modification time, so identical
// content always encodes to identical blocks, and a merge reuses each node it
// does not change.
package layer

import (
	"context"
	"io/fs"
	"path"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// Entry is one file, directory or symlink of a layer.
type Entry struct {
	// Path is the slash-separated path relative to the root, without a
	// leading slash.
	Path string
	// Type is fs.ModeDir, fs.ModeSymlink or 0 for a regular file.
	Type fs.FileMode
	// Perm holds the permission bits.
	Perm fs.FileMode
	// Data is the content of a regular file.
	Data []byte
	// Target is the destination of a symlink.
	Target string
}

// BaseEntries is the layer every root starts from: the top-level directories
// and the links that merge /bin, /lib and /sbin into /usr.
var BaseEntries = []*Entry{
	{Path: "dev", Type: fs.ModeDir, Perm: 0o755},
	{Path: "etc", Type: fs.ModeDir, Perm: 0o755},
	{Path: "proc", Type: fs.ModeDir, Perm: 0o555},
	{Path: "tmp", Type: fs.ModeDir, Perm: 0o777},
	{Path: "usr/bin", Type: fs.ModeDir, Perm: 0o755},
	{Path: "usr/lib", Type: fs.ModeDir, Perm: 0o755},
	{Path: "usr/sbin", Type: fs.ModeDir, Perm: 0o755},
	{Path: "var", Type: fs.ModeDir, Perm: 0o755},
	{Path: "bin", Type: fs.ModeSymlink, Target: "usr/bin"},
	{Path: "lib", Type: fs.ModeSymlink, Target: "usr/lib"},
	{Path: "sbin", Type: fs.ModeSymlink, Target: "usr/sbin"},
}

// epoch is the modification time of every layer node.
var epoch = &timestamp.Timestamp{Seconds: 1}

// dirType is the UnixFS node type of a directory.
const dirType = unixfs_block.NodeType_NodeType_DIRECTORY

// BuildTree writes a new UnixFS tree holding the entries, applied in order,
// through bls and returns its root reference.
func BuildTree(ctx context.Context, bls *bucket_lookup.Cursor, entries []*Entry) (*block.BlockRef, error) {
	// Start an empty root directory in a new transaction.
	btx, bcs := bls.BuildTransactionAtRef(nil, nil)
	bcs.SetBlock(unixfs_block.NewFSNode(dirType, 0o755, epoch), true)
	root, err := unixfs_block.NewFSTree(ctx, bcs, dirType)
	if err != nil {
		return nil, err
	}

	// Apply each entry and write the tree.
	for _, ent := range entries {
		if err := applyEntry(ctx, root, ent); err != nil {
			return nil, errors.Wrap(err, ent.Path)
		}
	}
	ref, _, err := btx.Write(ctx, true)
	return ref, err
}

// applyEntry creates one entry in the tree. A directory that exists is kept.
// Any other existing node at the path is replaced.
func applyEntry(ctx context.Context, root *unixfs_block.FSTree, ent *Entry) error {
	// Find or create the parent directory.
	parent, err := mkdirAll(root, path.Dir(ent.Path))
	if err != nil {
		return err
	}
	name := path.Base(ent.Path)

	// Keep an existing directory, and remove any other existing node.
	_, dirent, err := parent.LookupFollowDirent(name)
	if err != nil {
		return err
	}
	if dirent != nil {
		if ent.Type == fs.ModeDir && dirent.GetIsDirectory() {
			return nil
		}
		if _, err := parent.Remove([]string{name}, nil); err != nil {
			return err
		}
	}

	// Create the node by its type.
	switch ent.Type {
	case fs.ModeDir:
		_, err = parent.Mknod(name, dirType, nil, ent.Perm, epoch)
		return err
	case fs.ModeSymlink:
		lnk := &unixfs_block.FSSymlink{TargetPath: unixfs_block.SplitFSPath(ent.Target)}
		_, err = parent.Symlink(false, name, lnk, epoch)
		return err
	}
	node, err := parent.Mknod(name, unixfs_block.NodeType_NodeType_FILE, nil, ent.Perm, epoch)
	if err != nil {
		return err
	}
	return node.InitFileWithBytes(ctx, ent.Data)
}

// mkdirAll returns the directory at dir, creating missing directories with
// mode 0755.
func mkdirAll(root *unixfs_block.FSTree, dir string) (*unixfs_block.FSTree, error) {
	cur := root
	for _, name := range unixfs_block.SplitFSPath(dir).GetNodes() {
		next, err := cur.Mknod(name, dirType, nil, 0o755, epoch)
		if err != nil && !errors.Is(err, unixfs_errors.ErrExist) {
			return nil, err
		}
		if next.GetNodeType() != dirType {
			return nil, errors.Errorf("%s: not a directory", name)
		}
		cur = next
	}
	return cur, nil
}
