package unixfs_block_fs

import (
	"context"
	"io"
	"io/fs"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/file"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// FSCursorOps implements the filesystem ops against a fsTree instance.
type FSCursorOps struct {
	// isReleased indicates if this is released.
	isReleased atomic.Bool
	// cursor is the fs cursor
	cursor *FSCursor
	// fsTree is the filesystem tree
	fsTree *unixfs_block.FSTree
	// btx is the block transaction
	btx *block.Transaction
	// fileHandleMtx guards fileHandle
	fileHandleMtx sync.Mutex
	// fileHandle is the file handle if this is a file node
	fileHandle *file.Handle
}

// newFSCursorOps constructs a new FSCursorOps.
func newFSCursorOps(fsCursor *FSCursor, fsTree *unixfs_block.FSTree, btx *block.Transaction) *FSCursorOps {
	ops := &FSCursorOps{cursor: fsCursor, btx: btx, fsTree: fsTree}
	if ops.GetIsFile() {
		ops.fileHandle, _ = fsTree.BuildFileHandle(ops.cursor.fs.ctx)
	}
	return ops
}

// CheckReleased checks if the ops is released without locking anything.
func (f *FSCursorOps) CheckReleased() bool {
	if f == nil {
		return true
	}
	return f.isReleased.Load()
}

// GetName returns the name of the inode (if applicable).
// i.e. directory name, filename.
func (f *FSCursorOps) GetName() string {
	return f.cursor.name
}

// GetNodeType returns the node type of the inode.
func (f *FSCursorOps) GetNodeType() unixfs_block.NodeType {
	// note: changing the node type releases the ops object
	return f.fsTree.GetFSNode().GetNodeType()
}

// GetIsDirectory returns if the cursor points to a directory.
func (f *FSCursorOps) GetIsDirectory() bool {
	// note: changing the node type releases the ops object
	return f.fsTree.GetFSNode().GetNodeType().GetIsDirectory()
}

// GetIsFile returns if the cursor points to a file.
func (f *FSCursorOps) GetIsFile() bool {
	// note: changing the node type releases the ops object
	return f.fsTree.GetFSNode().GetNodeType().GetIsFile()
}

// GetIsSymlink returns if the cursor points to a symlink.
func (f *FSCursorOps) GetIsSymlink() bool {
	// note: changing the node type releases the ops object
	return f.fsTree.GetFSNode().GetNodeType().GetIsSymlink()
}

// GetSize returns the size of the inode (in bytes).
// Usually applicable only if this is a FILE.
func (f *FSCursorOps) GetSize(ctx context.Context) (uint64, error) {
	if f.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return f.fsTree.GetFSNode().GetFile().GetTotalSize(), nil
}

// GetModTimestamp returns the modification timestamp.
func (f *FSCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	if f.CheckReleased() {
		return time.Time{}, unixfs_errors.ErrReleased
	}
	return f.fsTree.GetFSNode().GetModTime().ToTime(), nil
}

// SetModTimestamp updates the modification timestamp of the node.
func (f *FSCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	writer := f.cursor.fs.writer
	if writer == nil {
		return unixfs_errors.ErrReadOnly
	}

	// call the writer to persist the change
	mpath, err := f.cursor.GetPath(ctx)
	if err != nil {
		return err
	}
	err = writer.SetModTimestamp(ctx, [][]string{mpath}, mtime)
	if err != nil {
		f.release()
		return err
	}

	return nil
}

// GetPermissions returns the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (f *FSCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	if f.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return f.fsTree.GetPermissions()
}

// SetPermissions sets the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (f *FSCursorOps) SetPermissions(ctx context.Context, fm fs.FileMode, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	writer := f.cursor.fs.writer
	if writer == nil {
		return unixfs_errors.ErrReadOnly
	}

	// call the writer to apply the changes
	npath, err := f.cursor.GetPath(ctx)
	if err != nil {
		return err
	}
	err = writer.SetPermissions(ctx, [][]string{npath}, fm, ts)
	if err != nil {
		f.release()
		return err
	}

	return nil
}

// ReadAt reads from an offset inside a file node.
func (f *FSCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	if f.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if f.fileHandle == nil {
		return 0, unixfs_errors.ErrNotFile
	}
	if f.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}

	// zero-size read
	if f.fileHandle.Size() == 0 {
		return 0, io.EOF
	}

	f.fileHandleMtx.Lock()
	defer f.fileHandleMtx.Unlock()

	idx, err := f.fileHandle.Seek(offset, io.SeekStart)
	if err == nil && idx < offset {
		err = io.EOF
	}
	if err != nil {
		return 0, err
	}

	n, err := io.ReadAtLeast(f.fileHandle, data, len(data))
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return int64(n), err
}

// GetOptimalWriteSize returns the best write size to use for the Write call.
// May return zero to indicate no known optimal size.
func (f *FSCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	// Use a constant target write size for the block filesystem (Blobs).
	return OptimalWriteSize, nil
}

// WriteAt writes to a location within a File node synchronously.
func (f *FSCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if f.fileHandle == nil {
		return unixfs_errors.ErrNotFile
	}
	writer := f.cursor.fs.writer
	if writer == nil {
		return unixfs_errors.ErrReadOnly
	}

	// call the writer to persist the changes.
	npath, err := f.cursor.GetPath(ctx)
	if err != nil {
		return err
	}
	err = writer.WriteAt(ctx, npath, offset, data, ts)
	if err != nil {
		// release this node because the state is now wrong.
		f.release()
		return err
	}

	return nil
}

// Truncate shrinks or extends a file to the specified size.
// The extended part will be a sparse range (hole) reading as zeros.
func (f *FSCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if f.fileHandle == nil {
		return unixfs_errors.ErrNotFile
	}
	writer := f.cursor.fs.writer
	if writer == nil {
		return unixfs_errors.ErrReadOnly
	}

	// call the writer to persist the changes
	npath, err := f.cursor.GetPath(ctx)
	if err != nil {
		return err
	}

	err = writer.Truncate(ctx, npath, int64(nsize), ts)
	if err != nil {
		f.release()
		return err
	}

	return nil
}

// Lookup looks up a child entry in a directory.
// Returns ErrNotExist if the child entry was not found.
// Returns ErrReleased if the reference has been released.
// Creates a new FSCursor at the new location.
func (f *FSCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	// Search for the entry
	childCs, dirent, err := f.fsTree.LookupFollowDirentAsCursor(name)
	if err != nil {
		return nil, err
	}
	if dirent == nil {
		return nil, unixfs_errors.ErrNotExist
	}

	// Add this inode
	return f.cursor.buildChildCursor(ctx, name, dirent, childCs)
}

// ReaddirAll reads all directory entries to a callback.
func (f *FSCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	dirStream, err := f.fsTree.Readdir()
	if err != nil {
		return err
	}
	if dirStream == nil {
		return nil
	}
	if skip > 0 {
		dirStream.Skip(int(skip))
	}
	for dirStream.Next() {
		ent := dirStream.GetEntry()
		if ent == nil {
			continue
		}
		if err := cb(ent); err != nil {
			return err
		}
	}

	return nil
}

// Mknod creates child entries in a directory.
// inode must be a directory.
// if checkExist, checks if name exists, returns ErrExist if so
func (f *FSCursorOps) Mknod(
	ctx context.Context,
	checkExist bool,
	names []string,
	nodeType unixfs.FSCursorNodeType,
	permissions fs.FileMode,
	ts time.Time,
) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	writer := f.cursor.fs.writer
	if writer == nil {
		return unixfs_errors.ErrReadOnly
	}

	// format change for the writer
	paths, err := f.buildChildPaths(ctx, names)
	if err != nil {
		return err
	}

	err = writer.Mknod(ctx, paths, nodeType, permissions, ts)
	if err != nil {
		f.release()
		return err
	}

	return nil
}

// Symlink creates a symbolic link from a location to a path.
func (f *FSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	writer := f.cursor.fs.writer
	if writer == nil {
		return unixfs_errors.ErrReadOnly
	}

	// call the writer to apply the change
	childPaths, err := f.buildChildPaths(ctx, []string{name})
	if err != nil {
		return err
	}
	childPath := childPaths[0]
	err = writer.Symlink(ctx, childPath, target, ts)
	if err != nil {
		f.release()
		return err
	}

	return nil
}

// Readlink reads a symbolic link contents.
// If name is empty, reads the link at the cursor position.
// Returns ErrNotSymlink if not a symbolic link.
func (f *FSCursorOps) Readlink(ctx context.Context, name string) ([]string, error) {
	var ftree *unixfs_block.FSTree
	if len(name) == 0 {
		ftree = f.fsTree
	} else {
		// lookup the entry
		nftree, dirent, err := f.fsTree.LookupFollowDirent(name)
		if err != nil {
			return nil, err
		}
		if dirent == nil {
			return nil, unixfs_errors.ErrNotExist
		}
		ftree = nftree
	}

	// verify that it is a symlink
	if ftree.GetFSNode().GetNodeType() != unixfs_block.NodeType_NodeType_SYMLINK {
		return nil, unixfs_errors.ErrNotSymlink
	}

	// return symlink value
	return ftree.GetFSNode().GetSymlink().GetTargetPath().GetNodes(), nil
}

// CopyTo performs an optimized copy of an dirent inode to another inode.
// If the src is a directory, this should be a recursive copy.
// Callers should still check CopyFrom even if CopyTo is not implemented.
// Returns false, nil if optimized copy to the target is not implemented.
func (f *FSCursorOps) CopyTo(
	ctx context.Context,
	tgtCursorOps unixfs.FSCursorOps,
	tgtName string,
	ts time.Time,
) (done bool, err error) {
	return f.moveOrCopyTo(ctx, tgtCursorOps, tgtName, false, ts)
}

// CopyFrom performs an optimized copy from another inode.
// If the src is a directory, this should be a recursive copy.
// Callers should still check CopyTo even if CopyFrom is not implemented.
// Returns false, nil if optimized copy from the target is not implemented.
func (f *FSCursorOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (done bool, err error) {
	// see CopyTo.
	return false, nil
}

// MoveTo performs an atomic and optimized move to another inode.
// If the src is a directory, this should be a recursive copy.
// Callers should still check MoveFrom even if MoveTo is not implemented.
//
// In a single operation: overwrite the target fully with the source data,
// and delete the source inode from its parent directory.
//
// Returns false, nil if atomic move to the target is not implemented.
func (f *FSCursorOps) MoveTo(
	ctx context.Context,
	tgtCursorOps unixfs.FSCursorOps,
	tgtName string,
	ts time.Time,
) (done bool, err error) {
	return f.moveOrCopyTo(ctx, tgtCursorOps, tgtName, true, ts)
}

// MoveFrom performs an atomic and optimized move from another inode.
// If the src is a directory, this should be a recursive copy.
// Callers should still check MoveTo even if MoveFrom is not implemented.
//
// In a single operation: overwrite the inode fully with the target data,
// and delete the target inode from its parent directory.
//
// Returns false, nil if atomic move from the target is not implemented.
func (f *FSCursorOps) MoveFrom(ctx context.Context, name string, tgtCursorOps unixfs.FSCursorOps, ts time.Time) (done bool, err error) {
	// see MoveTo.
	return false, nil
}

// moveOrCopyTo moves or copies a node to a destination.
// returns false, nil if not implemented or not possible.
// if move: updates the target of f to point to the new location.
func (f *FSCursorOps) moveOrCopyTo(
	ctx context.Context,
	tgtCursorOps unixfs.FSCursorOps,
	tgtName string,
	isMove bool,
	ts time.Time,
) (done bool, err error) {
	// XXX optimization: if both cursors have the same backing bucket and
	// transform config, copy the block ref over, clear cache, and release all
	// inodes below the move target (SetFSNode at target location).

	// if both cursors are from the same *FS, perform optimized copy/move operation.
	tgtOps, ok := tgtCursorOps.(*FSCursorOps)
	if !ok {
		return false, nil
	}

	tgtFsCursor := tgtOps.cursor
	fs := f.cursor.fs
	if tgtFsCursor.fs != fs {
		// different fs
		return false, nil
	}

	writer := f.cursor.fs.writer
	if writer == nil || writer != tgtFsCursor.fs.writer {
		// read-only or diff writer
		return false, nil
	}

	// build the paths
	srcPath, err := f.cursor.GetPath(ctx)
	if err != nil {
		return false, err
	}

	tgtParentPath, err := tgtOps.cursor.GetPath(ctx)
	if err != nil {
		return false, err
	}

	tgtPath := make([]string, len(tgtParentPath)+1)
	copy(tgtPath, tgtParentPath)
	tgtPath[len(tgtPath)-1] = tgtName

	// call the writer to persist the change
	// note: this one uses unixfs_block.CopyOrRename internally.
	if isMove {
		err = writer.Rename(ctx, srcPath, tgtPath, ts)
	} else {
		err = writer.Copy(ctx, srcPath, tgtPath, ts)
	}
	if err != nil {
		// we didn't change anything yet.
		return false, err
	}

	return true, nil
}

// Remove deletes entries from a directory.
// Returns ErrReadOnly if read-only.
func (f *FSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	tts := unixfs_block.ToTimestamp(ts, false)
	writer := f.cursor.fs.writer
	if writer == nil {
		return unixfs_errors.ErrReadOnly
	}

	paths, err := f.buildChildPaths(ctx, names)
	if err != nil {
		return err
	}

	err = writer.Remove(ctx, paths, ts)
	if err != nil {
		f.release()
		return err
	}

	if f.CheckReleased() {
		return nil
	}

	// apply the change to the local node
	if _, err := f.fsTree.Remove(names, tts); err != nil {
		f.release()
	}

	return nil
}

// buildChildPaths builds a list of paths rooted at f.
// pass empty (nil) to return a copy of the path to f.
// names must not be empty
// mtx must not be locked
func (f *FSCursorOps) buildChildPaths(ctx context.Context, names []string) ([][]string, error) {
	rootPath, err := f.cursor.GetPath(ctx)
	if err != nil {
		return nil, err
	}
	out := make([][]string, len(names))
	for i, name := range names {
		np := make([]string, len(rootPath)+1)
		copy(np, rootPath)
		np[len(np)-1] = name
		out[i] = np
	}
	return out, nil
}

// release marks the ops as released.
func (f *FSCursorOps) release() {
	if f.isReleased.Swap(true) {
		return
	}
	if f.fileHandle != nil {
		f.fileHandleMtx.Lock()
		_ = f.fileHandle.Close()
		f.fileHandleMtx.Unlock()
	}
}

// _ is a type assertion
var _ unixfs.FSCursorOps = ((*FSCursorOps)(nil))
