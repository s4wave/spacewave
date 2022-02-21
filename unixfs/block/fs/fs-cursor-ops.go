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
	// isReleased is an atomic int indicating released
	isReleased uint32
	// cursor is the fs cursor
	cursor *FSCursor
	// fsTree is the filesystem tree
	fsTree *unixfs_block.FSTree
	// btx is the block transaction
	btx *block.Transaction

	// sema is the semaphore for modifying below fields
	// XXX: use Sema to respect context cancels: sema *semaphore.Weighted
	mtx sync.Mutex
	// fileHandle is the file handle if this is a file node
	fileHandle *file.Handle
	// fileWriter is the file writer if this is a file node
	fileWriter *file.Writer
}

// newFSCursorOps constructs a new FSCursorOps.
func newFSCursorOps(fsCursor *FSCursor, fsTree *unixfs_block.FSTree, btx *block.Transaction) *FSCursorOps {
	ops := &FSCursorOps{cursor: fsCursor}
	ops.setFsTree(fsTree, btx)
	return ops
}

// setFsTree updates the fsTree, fileHandle, fileWriter, btx fields.
// expects to be in the constructor or have mtx locked
func (f *FSCursorOps) setFsTree(fsTree *unixfs_block.FSTree, btx *block.Transaction) {
	if f.fileWriter != nil {
		_ = f.fileWriter.Close()
	}
	if f.fileHandle != nil {
		_ = f.fileHandle.Close()
	}

	f.btx = btx
	f.fsTree = fsTree
	if f.GetIsFile() {
		f.fileHandle, _ = fsTree.BuildFileHandle(f.cursor.fs.ctx)
		f.fileWriter = file.NewWriter(f.fileHandle, nil, nil)
	}
}

// CheckReleased checks if the ops is released without locking anything.
func (f *FSCursorOps) CheckReleased() bool {
	if f == nil {
		return true
	}
	return atomic.LoadUint32(&f.isReleased) == 1
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
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return f.fsTree.GetFSNode().GetFile().GetTotalSize(), nil
}

// GetModTimestamp returns the modification timestamp.
func (f *FSCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.CheckReleased() {
		return time.Time{}, unixfs_errors.ErrReleased
	}
	return f.fsTree.GetFSNode().GetModTime().ToTime(), nil
}

// SetModTimestamp updates the modification timestamp of the node.
func (f *FSCursorOps) SetModTimestamp(ctx context.Context, t time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	writer := f.cursor.fs.writer
	if writer == nil {
		return unixfs_errors.ErrReadOnly
	}
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// allow a zero timestamp
	tts := unixfs_block.ToTimestamp(t, false)
	return f.fsTree.SetModTimestamp(tts)
}

// GetPermissions returns the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (f *FSCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
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
	// hold the sema
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	if err := f.fsTree.SetPermissions(fm); err != nil {
		return err
	}

	err := writer.SetPermissions(ctx, [][]string{f.cursor.GetPath()}, fm, ts)
	if err != nil {
		// release this node because the state is now wrong.
		f.release(false)
		return err
	}

	return nil
}

// Read reads from an offset inside a file node.
func (f *FSCursorOps) Read(ctx context.Context, offset int64, data []byte) (int64, error) {
	if f.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if f.fileHandle == nil {
		return 0, unixfs_errors.ErrNotFile
	}
	// hold the sema
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}

	// zero-size read
	if f.fileHandle.Size() == 0 {
		return 0, io.EOF
	}

	idx, err := f.fileHandle.Seek(offset, io.SeekStart)
	if err == nil && idx < offset {
		err = io.EOF
	}
	if err != nil {
		return 0, err
	}

	n, err := f.fileHandle.Read(data)
	return int64(n), err
}

// GetOptimalWriteSize returns the best write size to use for the Write call.
// May return zero to indicate no known optimal size.
func (f *FSCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	// Use a constant target write size for the block filesystem (Blobs).
	return OptimalWriteSize, nil
}

// Write writes to a location within a File node synchronously.
func (f *FSCursorOps) Write(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if f.fileHandle == nil || f.fileWriter == nil {
		return unixfs_errors.ErrNotFile
	}
	writer := f.cursor.fs.writer
	if writer == nil {
		return unixfs_errors.ErrReadOnly
	}

	// hold the sema
	f.mtx.Lock()
	defer f.mtx.Unlock()

	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// write to the in-memory block graph
	if err := f.fileWriter.WriteBytes(uint64(offset), data); err != nil {
		return err
	}
	// force a non-zero timestamp
	f.fsTree.GetFSNode().ModTime = unixfs_block.ToTimestamp(ts, true)

	// Call the writer to persist the changes.
	npath := f.cursor.GetPath()
	err := writer.Write(ctx, npath, offset, data, ts)
	if err != nil {
		// release this node because the state is now wrong.
		f.release(false)
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
	if f.fileHandle == nil || f.fileWriter == nil {
		return unixfs_errors.ErrNotFile
	}
	writer := f.cursor.fs.writer
	if writer == nil {
		return unixfs_errors.ErrReadOnly
	}

	// hold the sema
	f.mtx.Lock()
	defer f.mtx.Unlock()

	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// no-op same size
	if f.fileHandle.Size() == nsize {
		return nil
	}

	if err := f.fileWriter.Truncate(nsize); err != nil {
		return err
	}
	// force a non-zero timestamp
	f.fsTree.GetFSNode().ModTime = unixfs_block.ToTimestamp(ts, true)

	// Call the writer to persist the changes.
	npath := f.cursor.GetPath()
	if err := writer.Truncate(ctx, npath, int64(nsize), ts); err != nil {
		// release this node because the state is now wrong.
		f.release(false)
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

	// hold the sema
	f.mtx.Lock()
	defer f.mtx.Unlock()

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
	return f.cursor.buildChildCursor(name, dirent, childCs)
}

// ReaddirAll reads all directory entries to a callback.
func (f *FSCursorOps) ReaddirAll(ctx context.Context, cb func(ent unixfs.FSCursorDirent) error) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// hold the sema
	f.mtx.Lock()
	defer f.mtx.Unlock()

	dirStream, err := f.fsTree.Readdir()
	if err != nil {
		return err
	}
	if dirStream == nil {
		return nil
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

	f.mtx.Lock()
	defer f.mtx.Unlock()

	// apply the change to the local node first.
	nt := unixfs_block.FSCursorNodeTypeToNodeType(nodeType)
	var dirty bool
	// force a non-zero timestamp
	now := unixfs_block.ToTimestamp(ts, true)
	for _, name := range names {
		if name == "." {
			continue
		}

		_, err := f.fsTree.Mknod(name, nt, nil, permissions, now)
		if err == unixfs_errors.ErrExist && !checkExist {
			continue
		}
		if err != nil {
			if dirty {
				// undo our changes
				f.release(false)
			}
			return err
		}
		dirty = true
	}

	if !dirty {
		return nil
	}
	if err := f.flushChanges(); err != nil {
		return err
	}

	// format change for the writer
	paths := f.buildChildPaths(names)
	err := writer.Mknod(ctx, paths, nodeType, permissions, ts)
	if err != nil {
		// failed, revert this node
		f.release(false)
	}

	return err
}

// Symlink creates a symbolic link from a location to a path.
func (f *FSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	tts := unixfs_block.ToTimestamp(ts, false)
	tgtPath := unixfs_block.NewFSPath(target)
	nlnk := unixfs_block.NewFSSymlink(tgtPath)

	writer := f.cursor.fs.writer
	if writer == nil {
		return unixfs_errors.ErrReadOnly
	}

	f.mtx.Lock()
	defer f.mtx.Unlock()

	// apply the change to the local node
	_, err := f.fsTree.Symlink(checkExist, name, nlnk, tts)
	if err != nil {
		// undo changes
		f.release(false)
		return err
	}
	if err := f.flushChanges(); err != nil {
		return err
	}

	// format the change for the writer
	childPath := f.buildChildPaths([]string{name})[0]
	err = writer.Symlink(ctx, childPath, target, ts)
	if err != nil {
		f.release(false)
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

	// careful to lock in the correct order here
	// lock the entire *FS first
	f.cursor.fs.rmtx.Lock()
	defer f.cursor.fs.rmtx.Unlock()

	// lock the source cursor
	f.mtx.Lock()
	defer f.mtx.Unlock()

	// lock the target cursor
	tgtOps.mtx.Lock()
	defer tgtOps.mtx.Unlock()

	// check for released or inactive
	if f.CheckReleased() || tgtOps.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}
	if tgtOps.cursor.fsCursorOps != tgtOps || f.cursor.fsCursorOps != f {
		return false, nil
	}

	// check target is dir, return ErrNotDirectory otherwise
	if tgtOps.GetNodeType() != unixfs_block.NodeType_NodeType_DIRECTORY {
		return false, unixfs_errors.ErrNotDirectory
	}

	// srcBcs points to the source inode
	srcNodeType := f.GetNodeType()
	srcBcs := f.fsTree.GetCursor()

	// ensure the source is within a directory if isMove
	srcParentCursor := f.cursor.parent
	var srcParentOps *FSCursorOps
	if isMove {
		if err := srcParentCursor.resolveFsCursorOps(); err != nil {
			return false, err
		}
		srcParentOps = srcParentCursor.fsCursorOps
	}
	if srcParentOps == nil {
		// cannot move the root dir, at least not this way
		return false, nil
	}

	// access old dirent for the target if it exists
	tgtCs, _, err := tgtOps.fsTree.LookupFollowDirentAsCursor(tgtName)
	if err != nil {
		return false, err
	}

	// build the paths
	srcPath, tgtParentPath := f.cursor.getOrBuildPathLocked(), tgtOps.cursor.getOrBuildPathLocked()
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

	// copy to tgtCs or build new tgtCs
	if tgtCs == nil {
		// there was no existing dirent to overwrite, detach the parent
		tgtCs = tgtOps.fsTree.GetCursor().DetachTransaction()
	}

	srcBcs.CopyToRecursive(tgtCs, true, true)
	err = tgtOps.fsTree.SetDirent(tgtName, srcNodeType, tgtCs)
	if err != nil {
		return false, err
	}

	// fire the changed callbacks to update children states
	// because we updated the node in-place, the re-lookups will be against the new state
	// tgtCursor is the target location parent
	tgtCursor := tgtOps.cursor
	// tgtCursor.cbs = tgtCursor.cbs.CallCbs(&unixfs.FSCursorChange{Cursor: tgtOps.cursor})
	_ = tgtCursor

	// delete from source dir if move
	if isMove {
		tts := unixfs_block.ToTimestamp(ts, false)
		_, err := srcParentOps.fsTree.Remove([]string{f.cursor.name}, tts)
		if err != nil {
			srcParentOps.release(false)
		}
		if err != nil {
			return false, err
		}

		// srcParentCursor is the source location parent
		// srcParentCursor.cbs = srcParentCursor.cbs.CallCbs(&unixfs.FSCursorChange{Cursor: f.cursor})
	}

	// done
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

	f.mtx.Lock()
	defer f.mtx.Unlock()

	// apply the change to the local node
	_, err := f.fsTree.Remove(names, tts)
	if err != nil {
		// undo changes
		f.release(false)
		return err
	}

	// format the change for the writer
	paths := f.buildChildPaths(names)
	err = writer.Remove(ctx, paths, ts)
	if err != nil {
		// flush cache
		f.release(false)
	}
	// note: we don't need to flush cache if err = nil because we have applied
	// the removal locally already.

	return err
}

// flushChanges commits the block transaction.
// depends on sema being locked by caller
func (f *FSCursorOps) flushChanges() error {
	if f.btx == nil {
		// we must release to flush the cache right away.
		f.release(false)
		return nil
	}
	_, nrootCs, err := f.btx.Write(false)
	if err == nil {
		f.fsTree, err = unixfs_block.NewFSTree(nrootCs, f.fsTree.GetFSNode().GetNodeType())
	}
	if err != nil {
		// failed, revert this node
		f.release(false)
		return err
	}
	return nil
}

// buildChildPaths builds a list of paths rooted at f.
// pass empty (nil) to return a copy of the path to f.
// names must not be empty
func (f *FSCursorOps) buildChildPaths(names []string) [][]string {
	rootPath := f.cursor.GetPath()
	out := make([][]string, len(names))
	for i, name := range names {
		np := make([]string, len(rootPath)+1)
		copy(np, rootPath)
		np[len(np)-1] = name
		out[i] = np
	}
	return out
}

// release marks the fscursorops as released.
func (f *FSCursorOps) release(lockSema bool) {
	if lockSema {
		if f.CheckReleased() {
			return
		}
		f.mtx.Lock()
		defer f.mtx.Unlock()
	}
	if atomic.SwapUint32(&f.isReleased, 1) == 1 {
		return
	}
	if f.fileWriter != nil {
		_ = f.fileWriter.Close()
	}
	if f.fileHandle != nil {
		_ = f.fileHandle.Close()
	}
}

// _ is a type assertion
var _ unixfs.FSCursorOps = ((*FSCursorOps)(nil))
