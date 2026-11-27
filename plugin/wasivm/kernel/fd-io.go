package kernel

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"time"
)

// sysRead handles the read syscall. A blocking read returns EINTR when a
// signal is caught.
func (p *Process) sysRead(ctx context.Context, fd, bufPtr, count uint32) uint64 {
	// A read of nothing returns at once.
	if count == 0 {
		return 0
	}

	// Let a caught signal interrupt a blocking read.
	ctx, release := p.interruptible(ctx)
	defer release()

	// Read the descriptor into guest memory.
	of, ok := p.fds[fd]
	if !ok {
		return negErrno(EBADF)
	}
	data, errno := p.readFile(ctx, of, int(count))
	if errno != 0 {
		return errno
	}
	p.inst.Memory().Write(bufPtr, data)
	return uint64(len(data))
}

// readFile reads up to count bytes from a pipe, socket, event, timer or
// signal file, device or regular file, advancing a regular file's offset. An
// empty read of a pipe, socket, counter or signal file that is not at end of
// file returns EINTR when a signal interrupted it, or EAGAIN when the file is
// nonblocking.
func (p *Process) readFile(ctx context.Context, of *OpenFile, count int) ([]byte, uint64) {
	switch {
	case of.rx != nil:
		// Read a pipe, a connected socket or a pty master. A read drops the
		// files sent with the data, as on Linux.
		data, files, errno := p.readPipe(ctx, of, count)
		unrefFiles(files)
		return data, errno
	case of.counter != nil:
		return p.readCounter(ctx, of, count)
	case of.signals != nil:
		return p.readSignals(ctx, of, count)
	case of.tty != nil:
		// Read a terminal, which waits for input.
		buf := make([]byte, count)
		n, err := of.tty.in.read(of.ioContext(ctx), buf)
		return buf[:n], readStatus(ctx, of, n, err == io.EOF)
	case of.sock != nil:
		return nil, negErrno(ENOTCONN)
	case of.node == nil:
		return nil, negErrno(EBADF)
	}
	return p.readVFS(of, count)
}

// readPipe reads up to count bytes from the pipe of a pipe, connected socket
// or pty master, waiting for data unless the file is nonblocking. It returns
// the files sent with the first byte read, whose references pass to the
// caller.
func (p *Process) readPipe(ctx context.Context, of *OpenFile, count int) ([]byte, []*OpenFile, uint64) {
	buf := make([]byte, count)
	n, files, eof := of.rx.read(of.ioContext(ctx), buf)
	return buf[:n], files, readStatus(ctx, of, n, eof)
}

// readStatus returns the result of a read of a pipe, socket or terminal that
// took n bytes: success for data or end of file, EIO at the end of a pty
// master's slave, as Linux reports, and EINTR or EAGAIN for a read that took
// nothing.
func readStatus(ctx context.Context, of *OpenFile, n int, eof bool) uint64 {
	switch {
	case eof && n == 0 && of.ptm != nil:
		return negErrno(EIO)
	case n != 0 || eof:
		return 0
	case interrupted(ctx):
		return negErrno(EINTR)
	}
	return negErrno(EAGAIN)
}

// readVFS reads from a device or regular file at the descriptor's offset.
func (p *Process) readVFS(of *OpenFile, count int) ([]byte, uint64) {
	// Read a character device.
	if of.node.mode&sIFMT == sIFCHR {
		switch of.node.rdev {
		case devNull:
			return nil, 0
		case devZero:
			return make([]byte, count), 0
		case devURandom:
			buf := make([]byte, count)
			_, _ = rand.Read(buf)
			return buf, 0
		}
		return nil, negErrno(EIO)
	}

	// Read a regular file and advance the offset.
	data, errno := p.kernel.readNodeBytes(of, of.offset, count)
	if errno != 0 {
		return nil, errno
	}
	of.offset += int64(len(data))
	return data, 0
}

// sysWrite handles the write syscall.
func (p *Process) sysWrite(ctx context.Context, fd, bufPtr, count uint32) uint64 {
	buf, ok := p.inst.Memory().Read(bufPtr, count)
	if !ok {
		return negErrno(EFAULT)
	}
	return p.writeOutBytes(ctx, fd, buf)
}

// writeOutBytes writes buf to a descriptor's pipe, socket, terminal or file.
// write, writev and sendfile share it.
func (p *Process) writeOutBytes(ctx context.Context, fd uint32, buf []byte) uint64 {
	of, ok := p.fds[fd]
	switch {
	case !ok:
		return negErrno(EBADF)
	case of.tx != nil:
		return p.writePipe(ctx, of, buf, nil, false)
	case of.counter != nil:
		return p.writeCounter(ctx, of, buf)
	case of.signals != nil:
		return negErrno(EINVAL)
	case of.ptm != nil:
		return of.ptm.write(buf)
	case of.tty != nil:
		return p.writeTerminal(ctx, of, buf)
	case of.sock != nil:
		return negErrno(ENOTCONN)
	case of.node != nil:
		return p.writeVFS(of, buf)
	}
	return negErrno(EBADF)
}

// writePipe writes buf to a pipe or connected socket with files attached to
// its first byte, waiting while it is full. It takes over the caller's
// reference to each file, and drops them when no byte is written. A caught
// signal interrupts the wait, and a nonblocking file takes only what fits. A
// write with no reader fails with EPIPE, and raises SIGPIPE unless nosignal
// is set.
func (p *Process) writePipe(ctx context.Context, of *OpenFile, buf []byte, files []*OpenFile, nosignal bool) uint64 {
	// Write, waiting while the pipe is full.
	ctx, release := p.interruptible(ctx)
	defer release()
	n, ok := of.tx.write(of.ioContext(ctx), buf, files)
	if n == 0 {
		unrefFiles(files)
	}

	// Report a closed reader or a write that took nothing.
	switch {
	case !ok && nosignal:
		return negErrno(EPIPE)
	case !ok:
		p.deliverSignal(SIGPIPE)
		return negErrno(EPIPE)
	case n != 0 || len(buf) == 0:
		return uint64(n)
	case interrupted(ctx):
		return negErrno(EINTR)
	}
	return negErrno(EAGAIN)
}

// writeVFS writes to a device or regular file at the descriptor's offset.
func (p *Process) writeVFS(of *OpenFile, buf []byte) uint64 {
	// Write a character device.
	if of.node.mode&sIFMT == sIFCHR {
		switch of.node.rdev {
		case devNull:
			return uint64(len(buf))
		}
		return negErrno(EIO)
	}

	// Write a regular file the seals allow and advance the offset.
	if errno := of.node.checkWrite(of.offset, len(buf)); errno != 0 {
		return errno
	}
	n, errno := p.kernel.writeNodeBytes(of, of.offset, buf)
	if errno != 0 {
		return errno
	}
	of.offset += int64(n)
	return uint64(n)
}

// writeTerminal writes buf to a terminal as its output. A pty slave waits
// while its master's queue is full. A caught signal interrupts the wait, and
// a nonblocking file takes only what fits. A write after the master closed
// fails with EIO.
func (p *Process) writeTerminal(ctx context.Context, of *OpenFile, buf []byte) uint64 {
	// Write, waiting while the master's queue is full.
	ctx, release := p.interruptible(ctx)
	defer release()
	n, ok := of.tty.write(of.ioContext(ctx), buf, of.stderr)

	// Report a hangup or a write that took nothing.
	switch {
	case !ok:
		return negErrno(EIO)
	case n != 0 || len(buf) == 0:
		return uint64(n)
	case interrupted(ctx):
		return negErrno(EINTR)
	}
	return negErrno(EAGAIN)
}

// truncateNode resizes a file to size, zero-extending it or dropping its
// tail, unless a seal forbids it. truncate and ftruncate share it.
func (p *Process) truncateNode(node *VNode, size int64) uint64 {
	// Refuse a sealed change.
	if errno := node.checkTruncate(size); errno != 0 {
		return errno
	}

	// Resize a host file.
	if node.hostPath != "" {
		if err := p.kernel.vfs.truncateHost(node.hostPath, size); err != nil {
			return negErrno(EIO)
		}
		node.size = size
		return 0
	}

	// Resize an in-memory file.
	node.data = resize(node.data, size)
	node.size = size
	return 0
}

// resize returns data cut or zero-extended to size bytes.
func resize(data []byte, size int64) []byte {
	if size <= int64(len(data)) {
		return data[:size]
	}
	return append(data, make([]byte, size-int64(len(data)))...)
}

// readNodeBytes reads up to count bytes of an open file at off. It returns
// fewer bytes at the end of the file, or a negative errno. It does not move
// the descriptor offset.
func (k *Kernel) readNodeBytes(of *OpenFile, off int64, count int) ([]byte, uint64) {
	// Read a synthesized file's snapshot or a host file.
	node := of.node
	switch {
	case node == nil:
		return nil, negErrno(EBADF)
	case node.gen != nil:
		return readAt(of.data, off, count), 0
	case node.hostPath != "":
		f, err := k.vfs.host.Open(node.hostPath)
		if err != nil {
			return nil, negErrno(EIO)
		}
		defer f.Close()
		buf := make([]byte, count)
		n, _ := f.ReadAt(buf, off)
		return buf[:n], 0
	}

	// Copy from an in-memory file.
	return readAt(node.data[:node.size], off, count), 0
}

// readAt returns a copy of up to count bytes of data from off.
func readAt(data []byte, off int64, count int) []byte {
	if off >= int64(len(data)) {
		return nil
	}
	return bytes.Clone(data[off:min(off+int64(count), int64(len(data)))])
}

// writeNodeBytes writes data to an open file at off, growing the file as
// needed. It returns the bytes written or a negative errno. It does not move
// the descriptor offset.
func (k *Kernel) writeNodeBytes(of *OpenFile, off int64, data []byte) (int, uint64) {
	// Refuse a synthesized file, or write a host file and take its new size.
	node := of.node
	switch {
	case node == nil:
		return 0, negErrno(EBADF)
	case node.gen != nil:
		return 0, negErrno(EACCES)
	case node.hostPath != "":
		f, err := k.vfs.host.OpenFile(node.hostPath, os.O_WRONLY, 0)
		if err != nil {
			return 0, negErrno(EIO)
		}
		defer f.Close()
		n, err := f.WriteAt(data, off)
		if err != nil {
			return 0, negErrno(EIO)
		}
		if info, err := f.Stat(); err == nil {
			node.size = info.Size()
		}
		node.mtime = time.Now()
		return n, 0
	}

	// Write into an in-memory file.
	end := off + int64(len(data))
	if end > int64(len(node.data)) {
		node.data = resize(node.data, end)
	}
	copy(node.data[off:], data)
	node.size = max(node.size, end)
	node.mtime = time.Now()
	return len(data), 0
}
