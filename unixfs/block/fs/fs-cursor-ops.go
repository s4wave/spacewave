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
	// sema *semaphore.Weighted
	mtx sync.Mutex
	// fileHandle is the file handle if this is a file node
	fileHandle *file.Handle
	// fileWriter is the file writer if this is a file node
	fileWriter *file.Writer
}

// newFSCursorOps constructs a new FSCursorOps.
func newFSCursorOps(fsCursor *FSCursor, fsTree *unixfs_block.FSTree, btx *block.Transaction) *FSCursorOps {
	ops := &FSCursorOps{
		cursor: fsCursor,
		fsTree: fsTree,
		btx:    btx,
	}
	if ops.GetIsFile() {
		ops.fileHandle, _ = fsTree.BuildFileHandle(fsCursor.fs.ctx)
		ops.fileWriter = file.NewWriter(ops.fileHandle, nil, nil)
	}
	return ops
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
	return f.fsTree.GetFSNode().GetNodeType() == unixfs_block.NodeType_NodeType_DIRECTORY
}

// GetIsFile returns if the cursor points to a file.
func (f *FSCursorOps) GetIsFile() bool {
	// note: changing the node type releases the ops object
	return f.fsTree.GetFSNode().GetNodeType() == unixfs_block.NodeType_NodeType_FILE
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
		return 0, nil
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
	dirent, err := f.fsTree.Lookup(name)
	if err != nil {
		return nil, err
	}
	if dirent == nil {
		return nil, unixfs_errors.ErrNotExist
	}

	// Add this inode
	return f.cursor.buildChildCursor(name, dirent)
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

// Remove deletes entries from a directory.
// Returns ErrReadOnly if read-only.
func (f *FSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	writer := f.cursor.fs.writer
	if writer == nil {
		return unixfs_errors.ErrReadOnly
	}

	f.mtx.Lock()
	defer f.mtx.Unlock()

	// apply the change to the local node
	tts := unixfs_block.ToTimestamp(ts, false)
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
	if f.fileHandle != nil {
		_ = f.fileHandle.Close()
	}
}

// _ is a type assertion
var _ unixfs.FSCursorOps = ((*FSCursorOps)(nil))
