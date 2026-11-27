package kernel

import (
	"context"
	"sync"
)

// pipeCapacity is the size of a pipe's buffer, Linux's default.
const pipeCapacity = 65536

// pipeAtomic is the largest write a pipe never interleaves with another
// writer's, Linux's PIPE_BUF.
const pipeAtomic = 4096

// bufferedPipe is a pipe whose buffer holds up to pipeCapacity bytes. A
// writer waits while it is full, and a reader while it is empty. A pipe
// carries one direction of a pipe(2) pair or of a connected socket.
//
// A socket's pipe also carries open files, each list attached to the first
// byte of the write that sent it, as SCM_RIGHTS does on a Linux stream
// socket. A read never crosses into bytes that carry files, so the files
// arrive with the read that starts at their byte. A file in the pipe holds a
// reference, which the reader takes over. Files still in the pipe when its
// read side closes are released. A socket sent over itself keeps itself open
// until its peer closes, as on Linux without its garbage collector.
type bufferedPipe struct {
	// ino is the pipe's inode number.
	ino uint64

	// mtx guards the fields below.
	mtx sync.Mutex
	// buf holds the bytes written and not yet read.
	buf []byte
	// rights holds the files in flight, ordered by the offset in buf of the
	// byte each list is attached to.
	rights []pipeRights
	// readClosed is set when no open file reads the pipe.
	readClosed bool
	// writeClosed is set when no open file writes the pipe.
	writeClosed bool
	// waitCh closes when the buffer or the close state changes.
	waitCh chan struct{}
}

// pipeRights is a list of open files in flight on a pipe.
type pipeRights struct {
	// off is the offset in the pipe's buffer of the byte the files are
	// attached to.
	off int
	// files are the open files, each holding a reference.
	files []*OpenFile
}

// newBufferedPipe returns an empty pipe with inode number ino.
func newBufferedPipe(ino uint64) *bufferedPipe {
	return &bufferedPipe{ino: ino, waitCh: make(chan struct{})}
}

// read waits for data and copies it to dat, stopping before the next bytes
// that carry files. It returns the files attached to the first byte read,
// whose references pass to the caller, and reports true at end of file. When
// ctx ends first it returns 0 bytes without end of file, so a done ctx makes
// a read that does not wait.
func (p *bufferedPipe) read(ctx context.Context, dat []byte) (int, []*OpenFile, bool) {
	for {
		// Take buffered data, or end of file once the writers closed.
		p.mtx.Lock()
		if len(p.buf) != 0 {
			n, files := p.takeLocked(dat)
			p.notifyLocked()
			p.mtx.Unlock()
			return n, files, false
		}
		if p.writeClosed {
			p.mtx.Unlock()
			return 0, nil, true
		}
		waitCh := p.waitCh
		p.mtx.Unlock()

		// Wait for a write or a close.
		if !wait(ctx, waitCh) {
			return 0, nil, false
		}
	}
}

// takeLocked moves buffered bytes to dat up to the next bytes that carry
// files, and returns the count and the files attached to the first byte. The
// caller holds mtx and the buffer is not empty.
func (p *bufferedPipe) takeLocked(dat []byte) (int, []*OpenFile) {
	// Take the files at the first byte, and stop at the next list.
	var files []*OpenFile
	if len(p.rights) != 0 && p.rights[0].off == 0 {
		files = p.rights[0].files
		p.rights = p.rights[1:]
	}
	limit := len(p.buf)
	if len(p.rights) != 0 {
		limit = p.rights[0].off
	}

	// Copy the bytes and move the remaining lists' offsets with them.
	n := copy(dat, p.buf[:limit])
	p.buf = p.buf[n:]
	if len(p.buf) == 0 {
		p.buf = nil
	}
	for i := range p.rights {
		p.rights[i].off -= n
	}
	if len(p.rights) == 0 {
		p.rights = nil
	}
	return n, files
}

// write copies dat to the buffer, waiting for room. A write of at most
// pipeAtomic bytes waits until it fits whole. It attaches files to the first
// byte written, taking over their references. When nothing is written, as
// for empty dat, the caller keeps the references. It returns the bytes
// written, fewer when ctx ends, and reports false when the read side closed
// before any byte was written.
func (p *bufferedPipe) write(ctx context.Context, dat []byte, files []*OpenFile) (int, bool) {
	written := 0
	for {
		// Fail once no reader can receive the data.
		p.mtx.Lock()
		if p.readClosed {
			p.mtx.Unlock()
			return written, written != 0
		}

		// Copy what fits, with the files on the first byte.
		room := pipeCapacity - len(p.buf)
		if room > 0 && (len(dat) > pipeAtomic || room >= len(dat)) {
			if written == 0 && len(dat) != 0 && len(files) != 0 {
				p.rights = append(p.rights, pipeRights{off: len(p.buf), files: files})
			}
			n := min(room, len(dat))
			p.buf = append(p.buf, dat[:n]...)
			dat = dat[n:]
			written += n
			p.notifyLocked()
		}
		if len(dat) == 0 {
			p.mtx.Unlock()
			return written, true
		}
		waitCh := p.waitCh
		p.mtx.Unlock()

		// Wait for a read or a close.
		if !wait(ctx, waitCh) {
			return written, true
		}
	}
}

// state reports whether a read, or a write when write is set, would proceed
// without waiting, whether the other side has closed, and a channel that
// closes at the pipe's next change.
func (p *bufferedPipe) state(write bool) (ready, hangup bool, change <-chan struct{}) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if write {
		return p.readClosed || len(p.buf) < pipeCapacity, p.readClosed, p.waitCh
	}
	return len(p.buf) != 0 || p.writeClosed, p.writeClosed, p.waitCh
}

// close marks the read or write side closed and wakes the other side.
// Closing the read side releases the files in flight, so the caller holds
// the kernel lock when the pipe can carry files.
func (p *bufferedPipe) close(read bool) {
	// Mark the side closed, taking the files nobody will read.
	p.mtx.Lock()
	var dropped []pipeRights
	if read {
		p.readClosed = true
		p.buf = nil
		dropped, p.rights = p.rights, nil
	} else {
		p.writeClosed = true
	}
	p.notifyLocked()
	p.mtx.Unlock()

	// Release them outside the pipe's lock, since a file's release can
	// close this pipe's other side.
	for _, r := range dropped {
		unrefFiles(r.files)
	}
}

// reopenWrite marks the write side open again, for a pty slave opened after
// all its files closed.
func (p *bufferedPipe) reopenWrite() {
	p.mtx.Lock()
	p.writeClosed = false
	p.notifyLocked()
	p.mtx.Unlock()
}

// buffered returns the number of bytes a read could take now.
func (p *bufferedPipe) buffered() int {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	return len(p.buf)
}

// notifyLocked wakes the waiters on a change. The caller holds mtx.
func (p *bufferedPipe) notifyLocked() {
	close(p.waitCh)
	p.waitCh = make(chan struct{})
}
