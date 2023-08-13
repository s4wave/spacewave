package fuse

import (
	"context"
	"io"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	unixfs_block_fs "github.com/aperturerobotics/hydra/unixfs/block/fs"
	"github.com/aperturerobotics/util/csync"
)

// Handle wraps unixfs.InodeReference to provide FUSE file/dir handle calls.
// These include read/write/flush and optional buffering / blob behavior.
type Handle struct {
	inode     *Inode
	openFlags fuse.OpenFlags

	// rmtx guards below fields
	rmtx csync.RWMutex

	// writeSize is the minimum size of each write.
	// should be a multiple of 4096.
	// set using the GetOptimalWriteSize call on the inode
	writeSize uint64

	// writeErr contains any write error which will be returned and cleared on
	// next write attempt and/or flush.
	writeErr error

	// writeBuf is the buffer to write to before transmitting.
	writeBuf *pendingWrite

	// xmiting indicates if xmitBuf is currently being written
	// if nil, xmitBuf is not being written yet
	// if set and closed, xmitBuf was written or errored
	// if set and open, xmitBuf is being written
	xmiting chan struct{}
	// xmitBuf contains the data range currently being written
	xmitBuf *pendingWrite
}

// pendingWrite is a pending write buffer.
type pendingWrite struct {
	// offset is the location to write
	offset int64
	// buf is the buffer
	buf []byte
	// ts is the timestamp
	ts time.Time
}

// NewHandle constructs a new inode handle.
func NewHandle(inode *Inode, openFlags fuse.OpenFlags) *Handle {
	h := &Handle{
		inode:     inode,
		openFlags: openFlags,

		writeBuf: &pendingWrite{},
		xmitBuf:  &pendingWrite{},
	}
	adjustAddr := h.adjustInodeAttr
	inode.attrFn.Store(&adjustAddr)
	return h
}

// adjustInodeAttr is called by the inode in Attr.
func (h *Handle) adjustInodeAttr(ctx context.Context, attr *fuse.Attr) error {
	rel, err := h.rmtx.Lock(ctx, false)
	if err != nil {
		return err
	}
	defer rel()

	var writeBufEnd uint64
	if wb := h.writeBuf; wb != nil {
		writeBufEnd = uint64(wb.offset) + uint64(len(wb.buf))
	}
	var xmitBufEnd uint64
	if xb := h.xmitBuf; xb != nil {
		xmitBufEnd = uint64(xb.offset) + uint64(len(xb.buf))
	}

	fileSize := max(writeBufEnd, xmitBufEnd, attr.Size)
	if attr.Size != fileSize {
		attr.Size = fileSize

		// The blocks size must be calculated correctly:
		// (out.Size + blockSize - 1) / blockSize
		attr.Blocks = (fileSize + refBlockSize - 1) / refBlockSize
	}

	return nil
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
	var nread int64
	size, offset := int64(req.Size), req.Offset
	buf := make([]byte, int(size))

	rel, err := h.rmtx.Lock(ctx, false)
	if err != nil {
		return err
	}
	defer rel()

	if err != nil {
		h.inode.rfs.logFilesystemError(err)
		return UnixfsErrorToSyscall(err)
	}

	// Attempt to read from pending write buffers first.
	//  - writeBuf
	//  - xmitBuf

	pos := offset
	xbBuf, xbLen, xbOffset := h.xmitBuf, len(h.xmitBuf.buf), h.xmitBuf.offset
	writeBuf, wbLen, wbOffset := h.writeBuf, len(h.writeBuf.buf), h.writeBuf.offset
	xbEnd, wbEnd := int64(xbOffset)+int64(xbLen), int64(wbOffset)+int64(wbLen)
	if xbLen != 0 && pos >= int64(xbOffset) && pos < xbEnd {
		// read from xmit buf
		readFrom := xbBuf.buf[pos-xbOffset:]
		readLen := xbEnd - pos
		toRead := size - nread
		if readLen > toRead {
			readLen = toRead
		}
		copy(buf[nread:], readFrom[:readLen])
		nread += readLen
		pos += readLen
	}
	if wbLen != 0 && pos >= int64(wbOffset) && pos < wbEnd {
		// read from the write buf
		readFrom := writeBuf.buf[pos-wbOffset:]
		readLen := wbEnd - pos
		toRead := size - nread
		if readLen > toRead {
			readLen = toRead
		}
		copy(buf[nread:], readFrom[:readLen])
		nread += readLen
		pos += readLen
		// toRead -= readLen
	}

	// Read the remaining data directly from the inode.
	for nread < int64(size) {
		nr, err := h.inode.h.ReadAt(ctx, pos, buf[nread:])
		nread += nr
		pos += nr
		// ignore EOF or short buffer errors
		if nr == 0 || nread >= size || err == io.EOF {
			break
		}
		if err != nil {
			h.inode.rfs.logFilesystemError(err)
			return UnixfsErrorToSyscall(err)
		}
	}
	if nread > size {
		// not possible to read past end of the buffer
		nread = size
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
//
// TODO O_APPEND: ensure data always is appended to file
func (h *Handle) Write(
	ctx context.Context,
	req *fuse.WriteRequest,
	resp *fuse.WriteResponse,
) error {
	if h.openFlags.IsReadOnly() {
		return syscall.EROFS
	}

	isSync := h.openFlags&fuse.OpenSync != 0
	data, offset, ts := req.Data, req.Offset, time.Now()
	totalWriteSize := len(data)

	rel, err := h.rmtx.Lock(ctx, true)
	if err != nil {
		return err
	}
	defer func() {
		rel()
	}()

	// if O_SYNC is set, call Write directly
	if isSync {
		err := h.inode.h.WriteAt(ctx, offset, data, ts)
		if err != nil {
			h.inode.rfs.logFilesystemError(err)
			return UnixfsErrorToSyscall(err)
		}

		resp.Size = len(data)
		return nil
	}

	// waitTxFinish waits for the transmitting to finish
	// releases and re-acquires the semaphore
	waitTxFinish := func(xmiting chan struct{}) error {
		if xmiting == nil {
			return nil
		}
		rel()
		select {
		case <-ctx.Done():
			return UnixfsErrorToSyscall(ctx.Err())
		case <-xmiting:
		}
		// re-lock the sema before continuing
		rel, err = h.rmtx.Lock(ctx, true)
		if err != nil {
			h.inode.rfs.logFilesystemError(err)
			return UnixfsErrorToSyscall(err)
		}
		return nil
	}

	if h.writeErr != nil {
		// return deferred write error
		err := h.writeErr
		h.writeErr = nil
		h.inode.rfs.logFilesystemError(err)
		return UnixfsErrorToSyscall(err)
	}

	optimalWriteSize, err := h.getOptimalWriteSize(ctx)
	if err != nil {
		return err
	}

	// note: we update data slice bounds to only pending data
	for len(data) != 0 {
		// check if transmitting without swapping write buffer
		_ = h.checkOrStartXmit(false)

		// append data to write buffer & start transmitting if possible.
		// if the write is located at the end of write buf
		writeBufLen := len(h.writeBuf.buf)
		var writeBufPos int64
		if writeBufLen != 0 {
			writeBufPos = h.writeBuf.offset + int64(writeBufLen)
		} else {
			h.writeBuf.offset = offset
			h.writeBuf.ts = ts
			writeBufPos = offset
		}

		// we cannot write to writeBuf if the end of it != offset
		if writeBufPos != offset {
			// try to start transmitting right away
			_ = h.checkOrStartXmit(true)
			if len(h.writeBuf.buf) != 0 {
				// we need to wait for current transmission to finish.
				waitCh := h.xmiting
				if err := waitTxFinish(waitCh); err != nil {
					return err
				}
				continue
			}

			// use the newly zeroed write buf to write
			writeBufPos, h.writeBuf.offset = offset, offset
		}

		// copy data to writeBuf until it is at most optimalWriteSize
		extendWb := int(optimalWriteSize) - len(h.writeBuf.buf)
		if extendWb > 0 {
			if extendWb > len(data) {
				extendWb = len(data)
			}
			h.writeBuf.buf = append(h.writeBuf.buf, data[:extendWb]...)
			offset += int64(extendWb)
			data = data[extendWb:]
		}

		// transmit, swapping the buffers
		_ = h.checkOrStartXmit(true)

		// if we wrote everything, write is complete.
		if len(data) == 0 {
			break
		}

		// if writeBuf is empty, it must have been swapped to transmit.
		// continue right away
		if len(h.writeBuf.buf) == 0 {
			continue
		}

		// we need to wait for the current transmission to finish
		if waitCh := h.xmiting; waitCh != nil {
			if err := waitTxFinish(waitCh); err != nil {
				return err
			}
		}
	}

	resp.Size = totalWriteSize
	return nil
}

// checkOrStartXmit checks if transmitting, and starts transmitting if necessary.
// caller must lock sema
// returns if transmitting after the call
// if swapWriteBuf is set, may start transmitting by swapping writeBuf and xmitBuf.
func (h *Handle) checkOrStartXmit(swapWriteBuf bool) bool {
	if h.xmiting != nil {
		return true
	}

	if len(h.xmitBuf.buf) == 0 {
		if !swapWriteBuf || len(h.writeBuf.buf) == 0 {
			// nothing to do
			return false
		}

		// swap write and transmit bufs if there is data in the write buf
		xb := h.xmitBuf
		h.xmitBuf = h.writeBuf
		h.writeBuf = xb
	}

	// start transmit routine
	h.xmiting = make(chan struct{})
	go h.xmitData(h.xmitBuf, h.xmiting)
	return true
}

// xmitData is a goroutine started by checkOrStartXmit
func (h *Handle) xmitData(xmit *pendingWrite, xmiting chan struct{}) {
	ctx := h.inode.rfs.ctx
	xmitOffset, xmitBuf, xmitTs := xmit.offset, xmit.buf, xmit.ts
	err := h.inode.h.WriteAt(ctx, xmitOffset, xmitBuf, xmitTs)
	rel, lockErr := h.rmtx.Lock(context.Background(), true)
	if lockErr != nil {
		return
	}
	defer rel()
	if h.xmiting == xmiting {
		h.xmiting = nil
		h.xmitBuf.buf = h.xmitBuf.buf[:0]
		h.xmitBuf.offset = 0
		h.xmitBuf.ts = time.Time{}
		if err != nil {
			// clear write state if we encounter an error
			h.writeErr = err
			h.writeBuf.buf = nil
			h.writeBuf.offset = 0
			h.writeBuf.ts = time.Time{}
		}
	}
	// start next write cycle
	_ = h.checkOrStartXmit(true)
	rel() // release before close to prevent mtx contention
	close(xmiting)
}

// FlushWrites flushes all pending write data & waits for data write to complete.
func (h *Handle) FlushWrites(ctx context.Context) error {
	isSync := h.openFlags&fuse.OpenSync != 0
	if isSync {
		// all writes are sync: no need to flush
		return nil
	}

	// flush twice to ensure both writeBuf and xmitBuf are written
	for i := 0; i < 2; i++ {
		rel, err := h.rmtx.Lock(ctx, true)
		if err != nil {
			return err
		}
		// return any write error
		if err := h.writeErr; err != nil {
			h.writeErr = nil
			rel()
			return err
		}

		// start transmitting
		isXmit := h.checkOrStartXmit(true)
		if !isXmit {
			rel()
			break
		}

		// wait for transmit to complete
		xmiting := h.xmiting
		hasMore := len(h.writeBuf.buf) != 0
		rel()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-xmiting:
		}
		if !hasMore {
			break
		}
	}

	return nil
}

// Flush is called each time the file or directory is closed.
// Because there can be multiple file descriptors referring to a
// single opened file, Flush can be called multiple times.
func (h *Handle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	if err := h.FlushWrites(ctx); err != nil {
		return UnixfsErrorToSyscall(err)
	}

	return nil
}

// Release flushes and then closes the file handle.
// This does -not- forget the inode completely.
func (h *Handle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	if err := h.FlushWrites(ctx); err != nil {
		return UnixfsErrorToSyscall(err)
	}
	return nil
}

// getOptimalWriteSize returns the optimal xmit buf size.
// resolves it if not currently set
// caller must lock sema
func (h *Handle) getOptimalWriteSize(ctx context.Context) (uint64, error) {
	if h.writeSize != 0 {
		return h.writeSize, nil
	}
	optWriteSize, err := h.inode.h.GetOptimalWriteSize(ctx)
	if err != nil {
		return 0, err
	}
	if optWriteSize == 0 {
		optWriteSize = unixfs_block_fs.OptimalWriteSize
	}
	h.writeSize = uint64(optWriteSize)
	return uint64(optWriteSize), nil
}

// _ is a type assertion
var (
	_ fs.Handle = ((*Handle)(nil))

	_ fs.HandleReadDirAller = ((*Handle)(nil))
	_ fs.HandleReader       = ((*Handle)(nil))
	_ fs.HandleWriter       = ((*Handle)((nil)))
	_ fs.HandleReleaser     = ((*Handle)(nil))
	_ fs.HandleFlusher      = ((*Handle)(nil))
)
