package kernel

import (
	"cmp"
	"slices"
	"strings"
)

// The guest's mmap protection and sharing values the kernel reads.
const (
	guestProtWrite = 0x2
	guestMapShared = 0x1
	guestMapType   = 0x3
)

// The guest's memfd_create flags: close-on-exec, sealing and the executable
// bits.
const (
	guestMfdCloexec      = 0x1
	guestMfdAllowSealing = 0x2
	guestMfdNoexecSeal   = 0x8
	guestMfdExec         = 0x10
)

// mapping is a writable shared mapping of a file. Linear memory cannot map a
// file, so the C library's mmap allocates memory and the kernel fills it with
// the file's bytes. The kernel writes the memory back to the file at msync,
// munmap, exec and exit, and host code reads the file through the newest
// mapping's memory. The kernel lock guards it.
type mapping struct {
	// addr and length are the memory's range in the process.
	addr   uint32
	length uint32
	// of is the mapped file, holding a reference.
	of *OpenFile
	// off is the file offset of the memory's first byte.
	off int64
	// seq orders the kernel's mappings by creation. A later mapping's bytes
	// override an earlier one's.
	seq uint64
}

// overlaps reports whether the mapping's memory overlaps the length bytes at
// addr.
func (m *mapping) overlaps(addr, length uint32) bool {
	return uint64(m.addr) < uint64(addr)+uint64(length) && uint64(addr) < uint64(m.addr)+uint64(m.length)
}

// sysMemfdCreate creates an anonymous in-memory file and returns its
// descriptor. The name only labels the descriptor. MFD_ALLOW_SEALING leaves
// the file unsealed, and MFD_NOEXEC_SEAL also seals it unexecutable.
func (p *Process) sysMemfdCreate(namePtr, flags uint32) uint64 {
	// Check the flags.
	if flags&^(guestMfdCloexec|guestMfdAllowSealing|guestMfdNoexecSeal|guestMfdExec) != 0 {
		return negErrno(EINVAL)
	}
	name := readCString(p.inst.Memory(), namePtr)
	if strings.ContainsRune(name, '/') {
		return negErrno(EINVAL)
	}

	// Open a detached file, unsealed when the flags allow sealing.
	of := p.kernel.newMemfd(name)
	switch {
	case flags&guestMfdNoexecSeal != 0:
		of.node.mode &^= 0o111
		of.node.seals = sealExec
	case flags&guestMfdAllowSealing != 0:
		of.node.seals = 0
	}
	return uint64(p.installFD(of, flags&guestMfdCloexec != 0))
}

// newMemfd opens a detached in-memory file named for memfd_create. The
// caller holds the kernel lock and takes the first reference.
func (k *Kernel) newMemfd(name string) *OpenFile {
	node := k.vfs.newNode(sIFREG | 0o777)
	return &OpenFile{node: node, path: "/memfd:" + name + " (deleted)", flags: oRDWR}
}

// sysMmap fills the length bytes at addr with the file of fd from off, as the
// processes' mappings show it, zero past its end, for the C library's mmap of
// a file into memory it allocated. A writable shared mapping is recorded.
func (p *Process) sysMmap(addr, length, prot, flags, fd uint32, off int64) uint64 {
	// Check the file and the access a writable shared mapping needs.
	of, ok := p.fds[fd]
	shared := flags&guestMapType == guestMapShared && prot&guestProtWrite != 0
	switch {
	case !ok:
		return negErrno(EBADF)
	case of.node == nil || of.node.mode&sIFMT == sIFDIR:
		return negErrno(ENODEV)
	case off < 0 || uint64(addr)+uint64(length) > 1<<32:
		return negErrno(EINVAL)
	case of.flags&0o3 == oWRONLY || shared && of.flags&0o3 != oRDWR:
		return negErrno(EACCES)
	case shared && of.node.seals&(sealWrite|sealFutureWrite) != 0:
		return negErrno(EPERM)
	}

	// Copy the file into the memory.
	data, errno := p.kernel.readMapped(of, off, int(length))
	if errno != 0 {
		return errno
	}
	buf := make([]byte, length)
	copy(buf, data)
	if !p.inst.Memory().Write(addr, buf) {
		return negErrno(EFAULT)
	}

	// Record a writable shared mapping.
	if shared {
		of.ref()
		p.kernel.mapSeq++
		p.mappings = append(p.mappings, &mapping{
			addr:   addr,
			length: length,
			of:     of,
			off:    off,
			seq:    p.kernel.mapSeq,
		})
	}
	return 0
}

// sysMsync writes the memory of the writable shared mappings in the length
// bytes at addr back to their files.
func (p *Process) sysMsync(addr, length uint32) uint64 {
	for _, m := range p.mappings {
		if m.overlaps(addr, length) {
			p.writeBack(m)
		}
	}
	return 0
}

// sysMunmap writes back and forgets the writable shared mappings in the
// length bytes at addr. The C library frees the memory.
func (p *Process) sysMunmap(addr, length uint32) uint64 {
	p.mappings = slices.DeleteFunc(p.mappings, func(m *mapping) bool {
		if !m.overlaps(addr, length) {
			return false
		}
		p.writeBack(m)
		m.of.unref()
		return true
	})
	return 0
}

// unmapAll writes back and forgets every mapping, before the process's
// memory goes away at exec or exit. The caller holds the kernel lock.
func (p *Process) unmapAll() {
	for _, m := range p.mappings {
		p.writeBack(m)
		m.of.unref()
	}
	p.mappings = nil
}

// forkMappings gives a fork child, whose memory copies p's, the same
// mappings. The child's mappings are newer, so its writes override p's.
func (p *Process) forkMappings(child *Process) {
	for _, m := range p.mappings {
		m.of.ref()
		p.kernel.mapSeq++
		child.mappings = append(child.mappings, &mapping{
			addr:   m.addr,
			length: m.length,
			of:     m.of,
			off:    m.off,
			seq:    p.kernel.mapSeq,
		})
	}
}

// writeBack writes a mapping's memory to its file. A mapping never grows
// the file: the bytes past its end stay in memory, as a page past the end
// of a file does on Linux.
func (p *Process) writeBack(m *mapping) {
	size := p.kernel.fileSize(m.of.node)
	if m.off >= size {
		return
	}
	n := uint32(min(int64(m.length), size-m.off))
	if buf, ok := p.inst.Memory().Read(m.addr, n); ok {
		_, _ = p.kernel.writeNodeBytes(m.of, m.off, buf)
	}
}

// fileSize returns the size of a file node, refreshing a host file's size.
func (k *Kernel) fileSize(node *VNode) int64 {
	k.vfs.refreshHost(node)
	return node.size
}

// readMapped reads up to count bytes of the file of at off as the processes
// see it: the file's bytes, overlaid with the memory of each writable shared
// mapping of the file, oldest first. The caller holds the kernel lock.
func (k *Kernel) readMapped(of *OpenFile, off int64, count int) ([]byte, uint64) {
	// Read the file.
	data, errno := k.readNodeBytes(of, off, count)
	if errno != 0 || len(data) == 0 {
		return data, errno
	}

	// Find the mappings that overlap the range.
	end := off + int64(len(data))
	type overlay struct {
		p *Process
		m *mapping
	}
	var overlays []overlay
	for _, p := range k.procs.filter(func(p *Process) bool { return len(p.mappings) != 0 }) {
		for _, m := range p.mappings {
			if m.of.node == of.node && m.off < end && off < m.off+int64(m.length) {
				overlays = append(overlays, overlay{p, m})
			}
		}
	}
	slices.SortFunc(overlays, func(a, b overlay) int {
		return cmp.Compare(a.m.seq, b.m.seq)
	})

	// Copy each one's bytes over the file's.
	for _, o := range overlays {
		from, to := max(off, o.m.off), min(end, o.m.off+int64(o.m.length))
		buf, ok := o.p.inst.Memory().Read(o.m.addr+uint32(from-o.m.off), uint32(to-from))
		if ok {
			copy(data[from-off:], buf)
		}
	}
	return data, 0
}
