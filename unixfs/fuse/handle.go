package fuse

import (
	"context"
	"io"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// Handle wraps unixfs.InodeReference to provide FUSE file/dir handle calls.
// These include read/write/flush and optional buffering / blob behavior.
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

// Read requests to read data from the handle.
//
// There is a page cache in the kernel that normally submits only
// page-aligned reads spanning one or more pages. However, you
// should not rely on this. To see individual requests as
// submitted by the file system clients, set OpenDirectIO.
//
// Note that reads beyond the size of the file as reported by Attr
// are not even attempted (except in OpenDirectIO mode).
func (h *Handle) Read(
	ctx context.Context,
	req *fuse.ReadRequest,
	resp *fuse.ReadResponse,
) error {
	size, offset := int64(req.Size), req.Offset
	buf := make([]byte, int(size))
	var nread int64
	for nread < int64(size) {
		nr, err := h.inode.h.Read(ctx, offset+nread, buf[nread:])
		nread += nr
		if nread > size {
			// not possible to read past end of the buffer
			nread = size
			break
		}
		// ignore EOF or short buffer errors
		if nr == 0 || nread == size || err == io.EOF {
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

// Write requests to write data into the handle at the given offset.
// Store the amount of data written in resp.Size.
//
// There is a writeback page cache in the kernel that normally submits
// only page-aligned writes spanning one or more pages. However,
// you should not rely on this. To see individual requests as
// submitted by the file system clients, set OpenDirectIO.
//
// Writes that grow the file are expected to update the file size
// (as seen through Attr). Note that file size changes are
// communicated also through Setattr.
func (h *Handle) Write(
	ctx context.Context,
	req *fuse.WriteRequest,
	resp *fuse.WriteResponse,
) error {
	data, offset := req.Data, req.Offset
	// NOTE: this fully "flushes" and confirms the data.
	// the kernel will handle buffering
	ts := time.Now()
	err := h.inode.h.Write(ctx, offset, data, ts)
	if err != nil {
		h.inode.rfs.logFilesystemError(err)
		return UnixfsErrorToSyscall(err)
	}
	resp.Size = len(data)
	return nil
}

// Release flushes and then closes the file handle.
// This does -not- forget the inode completely.
func (h *Handle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	// NOTE currently all writes are flushed immediately so release is noop anyway.
	return nil
}

// _ is a type assertion
var (
	_ fs.Handle = ((*Handle)(nil))

	_ fs.HandleReadDirAller = ((*Handle)(nil))
	_ fs.HandleReader       = ((*Handle)(nil))
	_ fs.HandleWriter       = ((*Handle)((nil)))
	_ fs.HandleReleaser     = ((*Handle)(nil))
)
