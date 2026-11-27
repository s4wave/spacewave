package kernel

import (
	"testing"
)

// Guest memory addresses the mapping tests use.
const (
	mmapTestName = 1024
	mmapTestAddr = 8192
)

// memfdTest creates a memfd holding data and returns its descriptor.
func memfdTest(t *testing.T, p *Process, mem *memMemory, data string) uint32 {
	// Create the file and fill it.
	t.Helper()
	mem.writeCString(mmapTestName, "test")
	fd := p.sysMemfdCreate(mmapTestName, guestMfdCloexec|guestMfdAllowSealing)
	if errnoFailed(fd) {
		t.Fatalf("memfd_create = %#x", fd)
	}
	if _, errno := p.kernel.writeNodeBytes(p.fds[uint32(fd)], 0, []byte(data)); errno != 0 {
		t.Fatalf("write = %#x", errno)
	}
	return uint32(fd)
}

// hostFileTest returns a host reference to the file of fd.
func hostFileTest(p *Process, fd uint32) *File {
	of := p.fds[fd]
	of.ref()
	return &File{k: p.kernel, of: of}
}

// fileBytesTest reads the bytes stored in f's file, as a read would.
func fileBytesTest(t *testing.T, f *File) string {
	t.Helper()
	data, errno := f.k.readNodeBytes(f.of, 0, 64)
	if errno != 0 {
		t.Fatalf("read = %#x", errno)
	}
	return string(data)
}

// mappedBytesTest reads n bytes of f at off as the host does, through the
// mappings.
func mappedBytesTest(t *testing.T, f *File, off int64, n int) string {
	// Read and check the read filled the buffer.
	t.Helper()
	buf := make([]byte, n)
	got, err := f.ReadAt(buf, off)
	if err != nil {
		t.Fatalf("host ReadAt = %v", err)
	}
	return string(buf[:got])
}

// TestMmapShared checks a writable shared mapping of a memfd starts with the
// file's bytes, that the host and a new mapping read the program's writes
// through it before msync, and that msync and munmap write them back without
// growing the file, after the program closed its descriptor.
func TestMmapShared(t *testing.T) {
	// Map an 8-byte memfd into 16 bytes of memory.
	p, mem := fsTestProcess()
	fd := memfdTest(t, p, mem, "abcdefgh")
	f := hostFileTest(p, fd)
	defer f.Close()
	if res := p.sysMmap(mmapTestAddr, 16, guestProtWrite, guestMapShared, fd, 0); res != 0 {
		t.Fatalf("mmap = %#x", res)
	}
	if got, _ := mem.Read(mmapTestAddr, 16); string(got) != "abcdefgh\x00\x00\x00\x00\x00\x00\x00\x00" {
		t.Fatalf("mapped memory %q, want the file then zeros", got)
	}

	// The host and a new mapping read the program's write before msync.
	// Then the program closes its descriptor.
	mem.Write(mmapTestAddr+2, []byte("XYZ"))
	if got := mappedBytesTest(t, f, 1, 5); got != "bXYZf" {
		t.Fatalf("host read %q, want bXYZf", got)
	}
	if got := fileBytesTest(t, f); got != "abcdefgh" {
		t.Fatalf("file before msync %q, want abcdefgh", got)
	}
	if res := p.sysMmap(mmapTestAddr+64, 8, 0, 0x2, fd, 0); res != 0 {
		t.Fatalf("second mmap = %#x", res)
	}
	if got, _ := mem.Read(mmapTestAddr+64, 8); string(got) != "abXYZfgh" {
		t.Fatalf("second mapping %q, want abXYZfgh", got)
	}
	_ = p.sysClose(fd)

	// msync writes the memory back within the file's size.
	mem.Write(mmapTestAddr+8, []byte("past"))
	if res := p.sysMsync(mmapTestAddr, 16); res != 0 {
		t.Fatalf("msync = %#x", res)
	}
	if got := fileBytesTest(t, f); got != "abXYZfgh" {
		t.Fatalf("file after msync %q, want abXYZfgh", got)
	}

	// munmap writes the last change and forgets the mapping.
	mem.Write(mmapTestAddr, []byte("!"))
	if res := p.sysMunmap(mmapTestAddr, 16); res != 0 || len(p.mappings) != 0 {
		t.Fatalf("munmap = %#x with %d mappings, want none", res, len(p.mappings))
	}
	mem.Write(mmapTestAddr, []byte("?"))
	if got := mappedBytesTest(t, f, 0, 8); got != "!bXYZfgh" {
		t.Fatalf("host read after munmap %q, want !bXYZfgh", got)
	}
}

// TestMmapPrivate checks a private or read-only mapping copies the file and
// is not recorded, and that a write-only descriptor cannot be mapped.
func TestMmapPrivate(t *testing.T) {
	// Map privately and write the memory.
	p, mem := fsTestProcess()
	fd := memfdTest(t, p, mem, "abcd")
	f := hostFileTest(p, fd)
	defer f.Close()
	if res := p.sysMmap(mmapTestAddr, 4, guestProtWrite, 0x2, fd, 0); res != 0 {
		t.Fatalf("private mmap = %#x", res)
	}
	mem.Write(mmapTestAddr, []byte("Z"))
	if got := mappedBytesTest(t, f, 0, 4); got != "abcd" || len(p.mappings) != 0 {
		t.Fatalf("host read %q with %d mappings, want abcd and none", got, len(p.mappings))
	}

	// A write-only descriptor maps nothing.
	p.fds[fd].flags = oWRONLY
	if res := p.sysMmap(mmapTestAddr, 4, 0, guestMapShared, fd, 0); res != negErrno(EACCES) {
		t.Fatalf("mmap of a write-only descriptor = %#x, want EACCES", res)
	}
}

// TestMmapForkExit checks a fork child's copy of a mapping overrides the
// parent's for the host, and that exit writes each process's memory back
// and forgets its mappings.
func TestMmapForkExit(t *testing.T) {
	// Map a memfd.
	p, mem := fsTestProcess()
	fd := memfdTest(t, p, mem, "abcd")
	f := hostFileTest(p, fd)
	defer f.Close()
	if res := p.sysMmap(mmapTestAddr, 4, guestProtWrite, guestMapShared, fd, 0); res != 0 {
		t.Fatalf("mmap = %#x", res)
	}

	// Fork a child with a copy of the memory.
	childMem := newMemMemory(mem.Size())
	child := p.forkChild(2, memInstance{mem: childMem})
	if !copyInstance(child.inst, p.inst, nil) {
		t.Fatal("copy the instance")
	}
	p.kernel.procs.set(2, child)

	// The child's write wins over the parent's.
	mem.Write(mmapTestAddr, []byte("P"))
	childMem.Write(mmapTestAddr+1, []byte("C"))
	if got := mappedBytesTest(t, f, 0, 4); got != "aCcd" {
		t.Fatalf("host read %q, want the child's aCcd", got)
	}

	// The child's exit writes its memory back. The parent's exit follows.
	child.unmapAll()
	if got := fileBytesTest(t, f); got != "aCcd" {
		t.Fatalf("file after the child's exit %q, want aCcd", got)
	}
	if got := mappedBytesTest(t, f, 0, 4); got != "Pbcd" {
		t.Fatalf("host read after the child's exit %q, want the parent's Pbcd", got)
	}
	p.unmapAll()
	if got := fileBytesTest(t, f); got != "Pbcd" || len(p.mappings) != 0 {
		t.Fatalf("file after the parent's exit %q with %d mappings, want Pbcd and none", got, len(p.mappings))
	}
}

// TestMemfdCreateFlags checks memfd_create refuses unknown flags and a name
// holding a slash.
func TestMemfdCreateFlags(t *testing.T) {
	// Pass MFD_HUGETLB.
	p, mem := fsTestProcess()
	mem.writeCString(mmapTestName, "ok")
	if res := p.sysMemfdCreate(mmapTestName, 0x4); res != negErrno(EINVAL) {
		t.Fatalf("memfd_create with MFD_HUGETLB = %#x, want EINVAL", res)
	}

	// Name the file with a slash.
	mem.writeCString(mmapTestName, "a/b")
	if res := p.sysMemfdCreate(mmapTestName, 0); res != negErrno(EINVAL) {
		t.Fatalf("memfd_create of a/b = %#x, want EINVAL", res)
	}
}

// TestMemfdSeals checks a sealable memfd takes seals that refuse the changes
// they specify, that a writable shared mapping blocks sealWrite, and that a
// memfd created without MFD_ALLOW_SEALING takes none.
func TestMemfdSeals(t *testing.T) {
	// A mapped file cannot be write-sealed.
	p, mem := fsTestProcess()
	fd := memfdTest(t, p, mem, "abcd")
	of := p.fds[fd]
	if res := p.sysFcntl(fd, fGET_SEALS, 0); res != 0 {
		t.Fatalf("seals of a new memfd = %#x, want none", res)
	}
	p.sysMmap(mmapTestAddr, 4, guestProtWrite, guestMapShared, fd, 0)
	if res := p.sysFcntl(fd, fADD_SEALS, sealWrite); res != negErrno(EBUSY) {
		t.Fatalf("write seal while mapped = %#x, want EBUSY", res)
	}
	p.sysMunmap(mmapTestAddr, 4)

	// Size seals refuse resizes and writes past the end.
	if res := p.sysFcntl(fd, fADD_SEALS, sealShrink|sealGrow); res != 0 {
		t.Fatalf("add size seals = %#x", res)
	}
	if res := p.wasiFDFileStatSetSize(fd, 2); res != wasiErrnoFromResult(negErrno(EPERM)) {
		t.Fatalf("shrink = %d, want EPERM", res)
	}
	if res := p.wasiFDFileStatSetSize(fd, 8); res != wasiErrnoFromResult(negErrno(EPERM)) {
		t.Fatalf("grow = %d, want EPERM", res)
	}
	of.offset = 2
	if res := p.writeVFS(of, []byte("xy")); res != 2 {
		t.Fatalf("write within the file = %#x, want 2", res)
	}
	if res := p.writeVFS(of, []byte("z")); res != negErrno(EPERM) {
		t.Fatalf("write past the end = %#x, want EPERM", res)
	}

	// sealSeal refuses further seals.
	p.sysFcntl(fd, fADD_SEALS, sealSeal)
	if res := p.sysFcntl(fd, fADD_SEALS, sealWrite); res != negErrno(EPERM) {
		t.Fatalf("seal after sealSeal = %#x, want EPERM", res)
	}
	if res := p.sysFcntl(fd, fGET_SEALS, 0); res != sealSeal|sealShrink|sealGrow {
		t.Fatalf("seals = %#x", res)
	}

	// A memfd without MFD_ALLOW_SEALING holds sealSeal.
	mem.writeCString(mmapTestName, "fixed")
	fixed := uint32(p.sysMemfdCreate(mmapTestName, 0))
	if res := p.sysFcntl(fixed, fADD_SEALS, sealGrow); res != negErrno(EPERM) {
		t.Fatalf("seal of an unsealable memfd = %#x, want EPERM", res)
	}
}
