package unixfs_rpc_client

import (
	"context"
	"io"
	"io/fs"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	unixfs_rpc "github.com/aperturerobotics/hydra/unixfs/rpc"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// remoteFSCursorOps represents a remote FSCursorOps object.
type remoteFSCursorOps struct {
	released atomic.Bool
	c        *remoteFSCursor
	handleID uint64

	// FSCursorNodeType indicates the type of dirent.
	unixfs.FSCursorNodeType
	// name is the name of the dirent
	name string
	// nodeType is the original node type of the dirent
	nodeType unixfs_block.NodeType
	// optimalWriteSize is a cached value for optimal write size.
	// if zero the check will be run again.
	optimalWriteSize atomic.Int64
}

// newRemoteFSCursorOps constructs a new remoteFSCursorOps
func newRemoteFSCursorOps(
	c *remoteFSCursor,
	handleID uint64,
	nodeType unixfs_block.NodeType,
	name string,
) *remoteFSCursorOps {
	return &remoteFSCursorOps{
		c:                c,
		handleID:         handleID,
		FSCursorNodeType: nodeType,
		nodeType:         nodeType,
		name:             name,
	}
}

// CheckReleased implements unixfs.FSCursorOps.
func (o *remoteFSCursorOps) CheckReleased() bool {
	return o.released.Load() || o.c.released.Load()
}

// GetName returns the name of the inode (if applicable).
// i.e. directory name, filename.
func (o *remoteFSCursorOps) GetName() string {
	return o.name
}

// GetPermissions returns the permissions bits of the file mode.
// Only the permissions bits are set in the FileMode.
func (o *remoteFSCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}

	resp, err := o.c.c.client.OpsGetPermissions(ctx, &unixfs_rpc.OpsGetPermissionsRequest{
		OpsHandleId: o.handleID,
	})
	if err == nil {
		err = resp.GetUnixfsError().ToGoError()
	}
	if err != nil {
		o.handleErr(err, true)
		return 0, err
	}

	return fs.FileMode(resp.GetFileMode()), nil
}

// SetPermissions updates the permissions bits of the file mode.
// Only the permissions bits are used from the FileMode.
func (o *remoteFSCursorOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	tts := timestamp.ToTimestamp(ts)
	resp, err := o.c.c.client.OpsSetPermissions(ctx, &unixfs_rpc.OpsSetPermissionsRequest{
		OpsHandleId: o.handleID,
		FileMode:    uint32(permissions),
		Timestamp:   tts,
	})
	if err == nil {
		err = resp.GetUnixfsError().ToGoError()
	}
	if err != nil {
		o.handleErr(err, true)
		return err
	}
	return nil
}

// GetSize returns the size of the inode (in bytes).
// Usually applicable only if this is a FILE.
func (o *remoteFSCursorOps) GetSize(ctx context.Context) (uint64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}

	resp, err := o.c.c.client.OpsGetSize(ctx, &unixfs_rpc.OpsGetSizeRequest{
		OpsHandleId: o.handleID,
	})
	if err == nil {
		err = resp.GetUnixfsError().ToGoError()
	}
	if err != nil {
		o.handleErr(err, true)
		return 0, err
	}

	return resp.GetSize(), nil
}

// GetModTimestamp returns the modification timestamp.
func (o *remoteFSCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	if o.CheckReleased() {
		return time.Time{}, unixfs_errors.ErrReleased
	}

	resp, err := o.c.c.client.OpsGetModTimestamp(ctx, &unixfs_rpc.OpsGetModTimestampRequest{
		OpsHandleId: o.handleID,
	})
	if err == nil {
		err = resp.GetUnixfsError().ToGoError()
	}
	if err != nil {
		o.handleErr(err, true)
		return time.Time{}, err
	}

	return resp.GetModTimestamp().AsTime(), nil
}

// SetModTimestamp updates the modification timestamp of the node.
// mtime is the modification time to set.
func (o *remoteFSCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	mtimeTs := timestamp.ToTimestamp(mtime)
	resp, err := o.c.c.client.OpsSetModTimestamp(ctx, &unixfs_rpc.OpsSetModTimestampRequest{
		OpsHandleId:  o.handleID,
		ModTimestamp: mtimeTs,
	})
	if err == nil {
		err = resp.GetUnixfsError().ToGoError()
	}
	if err != nil {
		o.handleErr(err, true)
		return err
	}
	return nil
}

// ReadAt reads from a location in a File node.
// This is similar to ReadAt from io.ReaderAt.
//
// When ReadAt returns n < len(data), it returns a non-nil error explaining
// why more bytes were not returned. In this respect, ReadAt is stricter
// than Read.
//
// Even if ReadAt returns n < len(data), it may use all of p as scratch
// space during the call. If some data is available but not len(p) bytes,
// ReadAt blocks until either all the data is available or an error occurs.
// In this respect ReadAt is different from Read.
//
// If the n = len(data) bytes returned by ReadAt are at the end of the input
// source, ReadAt may return either err == EOF or err == nil.
//
// If ReadAt is reading from an input source with a seek offset, ReadAt
// should not affect nor be affected by the underlying seek offset.
//
// If this isn't a file node, returns ErrNotFile.
//
// Returns 0, io.EOF if the offset is past the end of the file.
// Returns the length read and any error.
func (o *remoteFSCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	if len(data) == 0 {
		return 0, io.ErrShortBuffer
	}
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}

	resp, err := o.c.c.client.OpsReadAt(ctx, &unixfs_rpc.OpsReadAtRequest{
		OpsHandleId: o.handleID,
		Offset:      offset,
		Size:        int64(len(data)),
	})
	if err == nil {
		err = resp.GetUnixfsError().ToGoError()
	}
	if err != nil && err != io.EOF {
		o.handleErr(err, true)
		return 0, err
	}

	retData := resp.GetData()
	if len(retData) < len(data) {
		data = data[:len(retData)]
	}
	copy(data, retData)
	return int64(len(data)), err
}

// GetOptimalWriteSize returns the best write size to use for the Write call.
// May return zero to indicate no known optimal size.
func (o *remoteFSCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	optimalWriteSize := o.optimalWriteSize.Load()
	if optimalWriteSize > 0 {
		return optimalWriteSize, nil
	}
	if optimalWriteSize == -1 {
		return 0, nil
	}
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}

	resp, err := o.c.c.client.OpsGetOptimalWriteSize(ctx, &unixfs_rpc.OpsGetOptimalWriteSizeRequest{
		OpsHandleId: o.handleID,
	})
	if err == nil {
		err = resp.GetUnixfsError().ToGoError()
	}
	if err != nil {
		o.handleErr(err, true)
		return 0, err
	}

	optimalWriteSize = resp.GetOptimalWriteSize()
	if optimalWriteSize > 0 {
		_ = o.optimalWriteSize.CompareAndSwap(0, optimalWriteSize)
	} else if optimalWriteSize == 0 {
		_ = o.optimalWriteSize.CompareAndSwap(0, -1)
	}

	return optimalWriteSize, nil
}

// WriteAt writes to a location within a File node synchronously.
// Accepts any size for the data parameter.
// Call GetOptimalWriteSize to determine the best size of data to use.
// The change should be fully written to the file before returning.
// If this isn't a file node, returns ErrNotFile.
func (o *remoteFSCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// NOTE: what if len(data) > max packet size for srpc?
	// we may want to enforce a size limit here & send multiple WriteAt packets.
	// or, alternatively return a written length from WriteAt (in case we write less).

	tts := timestamp.ToTimestamp(ts)
	resp, err := o.c.c.client.OpsWriteAt(ctx, &unixfs_rpc.OpsWriteAtRequest{
		OpsHandleId: o.handleID,
		Offset:      offset,
		Data:        data,
		Timestamp:   tts,
	})
	if err == nil {
		err = resp.GetUnixfsError().ToGoError()
	}
	if err != nil {
		o.handleErr(err, true)
		return err
	}

	return nil
}

// Truncate shrinks or extends a file to the specified size.
// The extended part will be a sparse range (hole) reading as zeros.
func (o *remoteFSCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	tts := timestamp.ToTimestamp(ts)
	resp, err := o.c.c.client.OpsTruncate(ctx, &unixfs_rpc.OpsTruncateRequest{
		OpsHandleId: o.handleID,
		Nsize:       nsize,
		Timestamp:   tts,
	})
	if err == nil {
		err = resp.GetUnixfsError().ToGoError()
	}
	if err != nil {
		o.handleErr(err, true)
		return err
	}

	return nil
}

// Lookup looks up a child entry in a directory.
// Returns ErrNotExist if the child entry was not found.
// Returns ErrReleased if the reference has been released.
// Creates a new FSCursor at the new location.
func (o *remoteFSCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	if o.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	resp, err := o.c.c.client.OpsLookup(ctx, &unixfs_rpc.OpsLookupRequest{
		OpsHandleId:    o.handleID,
		ClientHandleId: o.c.c.clientHandleID,
		CursorHandleId: o.c.cursorHandleID,
		Name:           name,
	})
	if err == nil {
		err = resp.GetUnixfsError().ToGoError()
	}

	o.c.c.mtx.Lock()
	defer o.c.c.mtx.Unlock()

	if o.c.c.released.Load() {
		return nil, unixfs_errors.ErrReleased
	}

	if err != nil {
		o.handleErr(err, false)
		return nil, err
	}

	handleID := resp.GetCursorHandleId()
	if handleID == 0 {
		return nil, unixfs_rpc.ErrHandleIDEmpty
	}

	retCursor := o.c.c.ingestCursorLocked(handleID)
	return retCursor, nil
}

// ReaddirAll reads all directory entries.
// If skip is set, skips the first N directory entries.
func (o *remoteFSCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	strm, err := o.c.c.client.OpsReaddirAll(ctx, &unixfs_rpc.OpsReaddirAllRequest{
		OpsHandleId: o.handleID,
		Skip:        skip,
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = strm.Close()
	}()

	for {
		resp, err := strm.Recv()
		if err != nil {
			return err
		}

		if err := resp.GetUnixfsError().ToGoError(); err != nil || resp.GetDone() {
			if err != nil {
				o.handleErr(err, true)
			}
			return err
		}

		if dirent := resp.GetDirent(); dirent.GetNodeType() != 0 {
			if err := cb(dirent); err != nil {
				return err
			}
		}
	}
}

// Mknod creates child entries in a directory.
// inode must be a directory.
// if permissions is zero, default permissions will be set.
// if checkExist, checks if name exists, returns ErrExist if so
func (o *remoteFSCursorOps) Mknod(
	ctx context.Context,
	checkExist bool,
	names []string,
	nodeType unixfs.FSCursorNodeType,
	permissions fs.FileMode,
	ts time.Time,
) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	tts := timestamp.ToTimestamp(ts)
	res, err := o.c.c.client.OpsMknod(ctx, &unixfs_rpc.OpsMknodRequest{
		OpsHandleId: o.handleID,
		CheckExist:  checkExist,
		Names:       names,
		NodeType:    unixfs_block.FSCursorNodeTypeToNodeType(nodeType),
		Permissions: uint32(permissions),
		Timestamp:   tts,
	})
	if err == nil {
		err = res.GetUnixfsError().ToGoError()
	}
	if err != nil {
		o.handleErr(err, true)
		return err
	}

	return nil
}

// Symlink creates a symbolic link from a location to a path.
func (o *remoteFSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, targetIsAbsolute bool, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	tts := timestamp.ToTimestamp(ts)
	res, err := o.c.c.client.OpsSymlink(ctx, &unixfs_rpc.OpsSymlinkRequest{
		OpsHandleId: o.handleID,
		CheckExist:  checkExist,
		Name:        name,
		Symlink:     unixfs_block.NewFSSymlink(unixfs_block.NewFSPath(target, targetIsAbsolute)),
		Timestamp:   tts,
	})
	if err == nil {
		err = res.GetUnixfsError().ToGoError()
	}
	if err != nil {
		o.handleErr(err, true)
		return err
	}

	return nil
}

// Readlink reads a symbolic link contents.
// If name is empty, reads the link at the cursor position.
// Returns ErrNotSymlink if not a symbolic link.
func (o *remoteFSCursorOps) Readlink(ctx context.Context, name string) ([]string, bool, error) {
	if o.CheckReleased() {
		return nil, false, unixfs_errors.ErrReleased
	}

	res, err := o.c.c.client.OpsReadlink(ctx, &unixfs_rpc.OpsReadlinkRequest{
		OpsHandleId: o.handleID,
		Name:        name,
	})
	if err == nil {
		err = res.GetUnixfsError().ToGoError()
	}
	if err != nil {
		o.handleErr(err, true)
		return nil, false, err
	}

	tgtPath := res.GetSymlink().GetTargetPath()
	return tgtPath.GetNodes(), tgtPath.GetAbsolute(), nil
}

// CopyTo performs an optimized copy of an dirent inode to another inode.
// If the src is a directory, this should be a recursive copy.
// Callers should still check CopyFrom even if CopyTo is not implemented.
// Returns false, nil if optimized copy to the target is not implemented.
func (o *remoteFSCursorOps) CopyTo(
	ctx context.Context,
	tgtDir unixfs.FSCursorOps,
	tgtName string,
	ts time.Time,
) (done bool, err error) {
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}

	// the target dir handle must also be a remote ops from the same client for
	// CopyTo to make sense. otherwise we will return false for optimized copy.
	tgtOpsHandle, ok := tgtDir.(*remoteFSCursorOps)
	if !ok || tgtOpsHandle == nil || tgtOpsHandle.c.c != o.c.c || tgtOpsHandle.CheckReleased() {
		return false, nil
	}

	tts := timestamp.ToTimestamp(ts)
	res, err := o.c.c.client.OpsCopyTo(ctx, &unixfs_rpc.OpsCopyToRequest{
		OpsHandleId:          o.handleID,
		TargetDirOpsHandleId: tgtOpsHandle.handleID,
		TargetName:           tgtName,
		Timestamp:            tts,
	})
	if err == nil {
		err = res.GetUnixfsError().ToGoError()
	}
	if err != nil {
		// Expire both the source and target ops since we can't distinguish which
		// cursor caused the error.
		tgtOpsHandle.handleErr(err, true)
		o.handleErr(err, true)
		return false, err
	}

	return res.GetDone(), nil
}

// CopyFrom performs an optimized copy from another inode.
// If the src is a directory, this should be a recursive copy.
// Callers should still check CopyTo even if CopyFrom is not implemented.
// Returns false, nil if optimized copy from the target is not implemented.
func (o *remoteFSCursorOps) CopyFrom(
	ctx context.Context,
	name string,
	srcCursorOps unixfs.FSCursorOps,
	ts time.Time,
) (done bool, err error) {
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}

	// the target dir handle must also be a remote ops from the same client for
	// CopyFrom to make sense. otherwise we will return false for optimized copy.
	srcOpsHandle, ok := srcCursorOps.(*remoteFSCursorOps)
	if !ok || srcOpsHandle == nil || srcOpsHandle.c.c != o.c.c || srcOpsHandle.CheckReleased() {
		return false, nil
	}

	tts := timestamp.ToTimestamp(ts)
	res, err := o.c.c.client.OpsCopyFrom(ctx, &unixfs_rpc.OpsCopyFromRequest{
		OpsHandleId:          o.handleID,
		Name:                 name,
		SrcCursorOpsHandleId: srcOpsHandle.handleID,
		Timestamp:            tts,
	})
	if err == nil {
		err = res.GetUnixfsError().ToGoError()
	}
	if err != nil {
		// Expire both the source and target ops since we can't distinguish which
		// cursor caused the error.
		srcOpsHandle.handleErr(err, true)
		o.handleErr(err, true)
		return false, err
	}

	return res.GetDone(), nil
}

// MoveTo performs an atomic and optimized move to another inode.
// If the src is a directory, this should be a recursive copy.
// Callers should still check MoveFrom even if MoveTo is not implemented.
//
// In a single operation: overwrite the target fully with the source data,
// and delete the source inode from its parent directory.
//
// Returns false, nil if atomic move to the target is not implemented.
func (o *remoteFSCursorOps) MoveTo(
	ctx context.Context,
	tgtCursorOps unixfs.FSCursorOps,
	tgtName string,
	ts time.Time,
) (done bool, err error) {
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}

	// the target dir handle must also be a remote ops from the same client for
	// MoveTo to make sense. otherwise we will return false for optimized move.
	tgtOpsHandle, ok := tgtCursorOps.(*remoteFSCursorOps)
	if !ok || tgtOpsHandle == nil || tgtOpsHandle.c.c != o.c.c || tgtOpsHandle.CheckReleased() {
		return false, nil
	}

	tts := timestamp.ToTimestamp(ts)
	res, err := o.c.c.client.OpsMoveTo(ctx, &unixfs_rpc.OpsMoveToRequest{
		OpsHandleId:          o.handleID,
		TargetDirOpsHandleId: tgtOpsHandle.handleID,
		TargetName:           tgtName,
		Timestamp:            tts,
	})
	if err == nil {
		err = res.GetUnixfsError().ToGoError()
	}
	if err != nil {
		// Expire both the source and target ops since we can't distinguish which
		// cursor caused the error.
		tgtOpsHandle.handleErr(err, true)
		o.handleErr(err, true)
		return false, err
	}

	return res.GetDone(), nil
}

// MoveFrom performs an atomic and optimized move from another inode.
// If the src is a directory, this should be a recursive copy.
// Callers should still check MoveTo even if MoveFrom is not implemented.
//
// In a single operation: overwrite the inode fully with the target data,
// and delete the target inode from its parent directory.
//
// Returns false, nil if atomic move from the target is not implemented.
func (o *remoteFSCursorOps) MoveFrom(
	ctx context.Context,
	name string,
	srcCursorOps unixfs.FSCursorOps,
	ts time.Time,
) (done bool, err error) {
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}

	// the target dir handle must also be a remote ops from the same client for
	// MoveFrom to make sense. otherwise we will return false for optimized move.
	srcOpsHandle, ok := srcCursorOps.(*remoteFSCursorOps)
	if !ok || srcOpsHandle == nil || srcOpsHandle.c.c != o.c.c || srcOpsHandle.CheckReleased() {
		return false, nil
	}

	tts := timestamp.ToTimestamp(ts)
	res, err := o.c.c.client.OpsMoveFrom(ctx, &unixfs_rpc.OpsMoveFromRequest{
		OpsHandleId:    o.handleID,
		Name:           name,
		SrcOpsHandleId: srcOpsHandle.handleID,
		Timestamp:      tts,
	})
	if err == nil {
		err = res.GetUnixfsError().ToGoError()
	}
	if err != nil {
		// Expire both the source and target ops since we can't distinguish which
		// cursor caused the error.
		srcOpsHandle.handleErr(err, true)
		o.handleErr(err, true)
		return false, err
	}

	return res.GetDone(), nil
}

// Remove deletes entries from a directory.
// Returns ErrReadOnly if read-only.
// Does not return an error if they did not exist.
func (o *remoteFSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	tts := timestamp.ToTimestamp(ts)
	res, err := o.c.c.client.OpsRemove(ctx, &unixfs_rpc.OpsRemoveRequest{
		OpsHandleId: o.handleID,
		Names:       names,
		Timestamp:   tts,
	})
	if err == nil {
		err = res.GetUnixfsError().ToGoError()
	}
	if err != nil {
		o.handleErr(err, true)
		return err
	}

	return nil
}

// MknodWithContent returns ErrReadOnly (RPC support not yet implemented).
func (o *remoteFSCursorOps) MknodWithContent(ctx context.Context, name string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// handleErr handles when an operations function returns an error.
func (o *remoteFSCursorOps) handleErr(err error, lock bool) {
	// mark the ops as released if we return ErrReleased.
	if err == unixfs_errors.ErrReleased {
		if lock {
			o.c.c.mtx.Lock()
			defer o.c.c.mtx.Unlock()
		}
		if !o.released.Swap(true) && o.c.c.ops[o.handleID] == o {
			delete(o.c.c.ops, o.handleID)
		}
	}
}

// _ is a type assertion
var _ unixfs.FSCursorOps = ((*remoteFSCursorOps)(nil))
