//go:build linux

package fuse

import (
	"context"
	"io"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// Handle wraps an Inode to provide FUSE file and directory handle calls.
//
// Reads and writes pass straight through to the inode, which buffers small
// contiguous writes. Flush, called on every close, and Fsync commit them, so
// close reports a write that failed to commit.
type Handle struct {
	inode     *Inode
	openFlags fuse.OpenFlags
}

// NewHandle constructs a new inode handle.
func NewHandle(inode *Inode, openFlags fuse.OpenFlags) *Handle {
	return &Handle{inode: inode, openFlags: openFlags}
}

// ReadDirAll handles the readdir call.
func (h *Handle) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	return h.inode.ReadDirAll(ctx)
}

// Read reads up to req.Size bytes at req.Offset, returning fewer at the end
// of the file.
func (h *Handle) Read(
	ctx context.Context,
	req *fuse.ReadRequest,
	resp *fuse.ReadResponse,
) error {
	// Read until the buffer is full, the file ends, or a read fails.
	buf := make([]byte, req.Size)
	var nread int
	for nread < len(buf) {
		nr, err := h.inode.h.ReadAt(ctx, req.Offset+int64(nread), buf[nread:])
		nread += int(nr)
		if err == io.EOF || (err == nil && nr == 0) {
			break
		}
		if err != nil {
			h.inode.rfs.logFilesystemError(err)
			return UnixfsErrorToSyscall(err)
		}
	}
	resp.Data = buf[:nread]
	return nil
}

// Write writes req.Data at req.Offset. A small write may return before it
// commits; Flush and Fsync commit it.
//
// TODO O_APPEND: ensure data always is appended to file
func (h *Handle) Write(
	ctx context.Context,
	req *fuse.WriteRequest,
	resp *fuse.WriteResponse,
) error {
	// Reject writes to a read-only handle.
	if h.openFlags.IsReadOnly() {
		return syscall.EROFS
	}

	// Write through the inode.
	if err := h.inode.h.WriteAt(ctx, req.Offset, req.Data, time.Now()); err != nil {
		h.inode.rfs.logFilesystemError(err)
		return UnixfsErrorToSyscall(err)
	}
	resp.Size = len(req.Data)
	return nil
}

// Flush commits buffered writes when a file descriptor closes.
func (h *Handle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	if err := h.inode.h.Sync(ctx); err != nil {
		h.inode.rfs.logFilesystemError(err)
		return UnixfsErrorToSyscall(err)
	}
	return nil
}

// _ is a type assertion
var (
	_ fs.Handle = (*Handle)(nil)

	_ fs.HandleFlusher      = (*Handle)(nil)
	_ fs.HandleReadDirAller = (*Handle)(nil)
	_ fs.HandleReader       = (*Handle)(nil)
	_ fs.HandleWriter       = (*Handle)(nil)
)
