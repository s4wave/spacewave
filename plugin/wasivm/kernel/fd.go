package kernel

import "os"

// openResolved opens an already-resolved VFS path and installs it in the
// process fd table, close-on-exec with O_CLOEXEC. POSIX openat and WASI
// path_open share this owner path so descriptors are readable, seekable, and
// closeable through either ABI.
func (p *Process) openResolved(resolved string, flags, mode uint32) uint64 {
	// Take O_CLOEXEC, which marks the descriptor rather than the file.
	cloexec := flags&oCLOEXEC != 0
	flags &^= oCLOEXEC

	// Find the file, creating it when asked.
	node := p.kernel.vfs.lookup(resolved)
	switch {
	case node == nil && flags&oCREAT == 0:
		return negErrno(ENOENT)
	case node == nil:
		var errno uint64
		if node, errno = p.createFile(resolved, flags, mode&0o7777); errno != 0 {
			return errno
		}
	case flags&oCREAT != 0 && flags&oEXCL != 0:
		return negErrno(EEXIST)
	}

	// Check the type and apply truncation.
	if flags&oDIRECTORY != 0 && node.mode&sIFMT != sIFDIR {
		return negErrno(ENOTDIR)
	}
	if flags&oTRUNC != 0 && node.mode&sIFMT == sIFREG {
		_ = p.truncateNode(node, 0)
	}

	// Open a terminal device.
	of := &OpenFile{node: node, path: resolved, flags: flags}
	if node.mode&sIFMT == sIFCHR {
		if node.rdev == devPtmx {
			return p.openPtmx(flags, cloexec)
		}
		if errno := p.openTerminal(of); errno != 0 {
			return errno
		}
	}

	// Install the descriptor, starting an appending file at its end. A
	// synthesized file reads the content it has at the open.
	if node.gen != nil {
		of.data = node.gen()
	}
	if flags&oAPPEND != 0 {
		p.kernel.vfs.refreshHost(node)
		of.offset = node.size
	}
	return uint64(p.installFD(of, cloexec))
}

// openTerminal binds an open of /dev/tty, /dev/console or a pty slave to its
// terminal. /dev/tty selects p's controlling terminal. As on Linux, a session
// leader without a controlling terminal that opens a pty slave no session
// controls, without O_NOCTTY, takes it as its controlling terminal.
func (p *Process) openTerminal(of *OpenFile) uint64 {
	// Find the terminal.
	t := of.node.tty
	if of.node.rdev == devTTY {
		if t = p.terminal(); t == nil {
			return negErrno(ENXIO)
		}
	}

	// Count an open of a slave, and take it as the controlling terminal.
	if t != nil && t.pty != nil {
		if errno := t.pty.openSlave(); errno != 0 {
			return errno
		}
		if of.flags&oNOCTTY == 0 && p.sid == p.pid && p.ctty == nil && t.sid == 0 {
			p.control(t)
		}
	}
	of.tty = t
	return 0
}

// createFile creates a regular file at a resolved path whose parent
// directory exists. A file in a host-backed directory is created on the host.
func (p *Process) createFile(resolved string, flags, perm uint32) (*VNode, uint64) {
	// Find the parent directory.
	vfs := p.kernel.vfs
	dir, name := splitPath(resolved)
	parent := vfs.lookup(dir)
	if parent == nil || parent.mode&sIFMT != sIFDIR {
		return nil, negErrno(ENOENT)
	}
	if parent.hostPath == "" {
		return vfs.mkFile(parent, name, nil, perm), 0
	}

	// Create the host file.
	hostPath := hostChildPath(parent.hostPath, name)
	osFlags := os.O_CREATE | os.O_RDWR
	if flags&oEXCL != 0 {
		osFlags |= os.O_EXCL
	}
	f, err := vfs.host.OpenFile(hostPath, osFlags, os.FileMode(perm))
	if err != nil {
		return nil, negErrno(EIO)
	}
	_ = f.Close()

	// Add its node.
	node := vfs.mkFile(parent, name, nil, perm)
	node.hostPath = hostPath
	return node, 0
}

// sysClose closes a file descriptor.
func (p *Process) sysClose(fd uint32) uint64 {
	// Drop the descriptor's reference to its open file.
	of, ok := p.fds[fd]
	if !ok {
		return negErrno(EBADF)
	}
	of.unref()
	delete(p.fds, fd)
	delete(p.cloexec, fd)
	return 0
}

// installFD points the lowest unused descriptor at of, close-on-exec when
// cloexec is set, and returns it.
func (p *Process) installFD(of *OpenFile, cloexec bool) uint32 {
	fd := p.allocFD()
	p.setFD(fd, of)
	p.setCloexec(fd, cloexec)
	return fd
}

// setFD points descriptor fd at of, closing the file fd held before. The
// descriptor starts without close-on-exec.
func (p *Process) setFD(fd uint32, of *OpenFile) {
	of.ref()
	if old, ok := p.fds[fd]; ok {
		old.unref()
	}
	p.fds[fd] = of
	delete(p.cloexec, fd)
}

// setCloexec sets or clears the close-on-exec flag of descriptor fd.
func (p *Process) setCloexec(fd uint32, cloexec bool) {
	if cloexec {
		p.cloexec[fd] = struct{}{}
	} else {
		delete(p.cloexec, fd)
	}
}

// sysFcntl handles file control operations. The descriptor flags hold only
// FD_CLOEXEC. F_GETFL and F_SETFL take the file status flags in the guest's
// values, and F_SETFL changes only the append and nonblocking flags, as on
// Linux.
func (p *Process) sysFcntl(fd, cmd, arg uint32) uint64 {
	of, ok := p.fds[fd]
	if !ok {
		return negErrno(EBADF)
	}
	switch cmd {
	case fDUPFD, fDUPFD_CLOEXEC:
		newFD := arg
		for p.fds[newFD] != nil {
			newFD++
		}
		p.setFD(newFD, of)
		p.setCloexec(newFD, cmd == fDUPFD_CLOEXEC)
		return uint64(newFD)
	case fGETFD:
		if _, ok := p.cloexec[fd]; ok {
			return fdCLOEXEC
		}
		return 0
	case fSETFD:
		p.setCloexec(fd, arg&fdCLOEXEC != 0)
		return 0
	case fGETFL:
		return uint64(guestFileFlags(of.flags))
	case fSETFL:
		of.flags = of.flags&^(oAPPEND|oNONBLOCK) | kernelStatusFlags(arg)
		return 0
	case fADD_SEALS:
		return p.kernel.addSeals(of, arg)
	case fGET_SEALS:
		return getSeals(of)
	}
	return negErrno(EINVAL)
}

// guestFileFlags translates a file's access mode and status flags to the
// guest's values.
func guestFileFlags(flags uint32) uint32 {
	// Translate the access mode.
	var guest uint32
	switch flags & 0o3 {
	case oRDONLY:
		guest = guestORDONLY
	case oWRONLY:
		guest = guestOWRONLY
	default:
		guest = guestORDONLY | guestOWRONLY
	}

	// Add the status flags.
	if flags&oAPPEND != 0 {
		guest |= guestOAPPEND
	}
	if flags&oNONBLOCK != 0 {
		guest |= guestONONBLOCK
	}
	return guest
}

// kernelStatusFlags translates the append and nonblocking flags from the
// guest's values to the kernel's.
func kernelStatusFlags(guest uint32) uint32 {
	var flags uint32
	if guest&guestOAPPEND != 0 {
		flags |= oAPPEND
	}
	if guest&guestONONBLOCK != 0 {
		flags |= oNONBLOCK
	}
	return flags
}

// sysDup duplicates a file descriptor to the next free number.
func (p *Process) sysDup(fd uint32) uint64 {
	// Share the open file under a new number.
	of, ok := p.fds[fd]
	if !ok {
		return negErrno(EBADF)
	}
	newFD := p.allocFD()
	p.setFD(newFD, of)
	return uint64(newFD)
}

// sysDup2 duplicates a file descriptor to a specific number. Duplicating a
// descriptor onto itself changes nothing.
func (p *Process) sysDup2(oldfd, newfd uint32) uint64 {
	of, ok := p.fds[oldfd]
	if !ok {
		return negErrno(EBADF)
	}
	if oldfd != newfd {
		p.setFD(newfd, of)
	}
	return uint64(newfd)
}

// fdTerminal returns the terminal of descriptor fd, or nil when fd is not a
// terminal.
func (p *Process) fdTerminal(fd uint32) *tty {
	of, ok := p.fds[fd]
	if !ok {
		return nil
	}
	return of.terminal()
}

// sysPipe2 creates a pipe and writes its read and write descriptors to
// pipefdPtr. flags holds the guest's status flags and O_CLOEXEC for both
// ends.
func (p *Process) sysPipe2(pipefdPtr, flags uint32) uint64 {
	// Install both ends.
	pipe := newBufferedPipe(p.kernel.vfs.allocIno())
	status, cloexec := kernelStatusFlags(flags), flags&guestOCLOEXEC != 0
	rfd := p.installFD(&OpenFile{rx: pipe, flags: oRDONLY | status}, cloexec)
	wfd := p.installFD(&OpenFile{tx: pipe, flags: oWRONLY | status}, cloexec)

	// Return their numbers.
	mem := p.inst.Memory()
	mem.WriteUint32Le(pipefdPtr, rfd)
	mem.WriteUint32Le(pipefdPtr+4, wfd)
	return 0
}

// sysFchdir changes the current working directory to an open directory.
func (p *Process) sysFchdir(fd uint32) uint64 {
	of, ok := p.fds[fd]
	if !ok || of.path == "" {
		return negErrno(EBADF)
	}
	return p.chdirResolved(of.path)
}

// sysFsync is a no-op: the in-memory VFS has nothing to flush, and host
// writes go straight to the host file.
func (p *Process) sysFsync(fd uint32) uint64 {
	if _, ok := p.fds[fd]; !ok {
		return negErrno(EBADF)
	}
	return 0
}
