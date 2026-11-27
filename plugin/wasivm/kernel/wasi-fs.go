package kernel

import (
	"cmp"
	"context"
	"encoding/binary"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// Flags of the WASI path and descriptor calls.
const (
	wasiLookupSymlinkFollow = 1

	wasiOflagsCreat     = 1
	wasiOflagsDirectory = 2
	wasiOflagsExcl      = 4
	wasiOflagsTrunc     = 8

	wasiFdflagsAppend   = 1
	wasiFdflagsNonblock = 4
	// wasiFdflagsNoctty and wasiFdflagsCloexec are the WasiVM libc's
	// O_NOCTTY and O_CLOEXEC, which its open passes in bits of the fdflags
	// WASI leaves unused.
	wasiFdflagsNoctty  = 0x100
	wasiFdflagsCloexec = guestOCLOEXEC

	wasiRiflagsRecvPeek = 1

	wasiFilestatSetAtim    = 1
	wasiFilestatSetAtimNow = 2
	wasiFilestatSetMtim    = 4
	wasiFilestatSetMtimNow = 8

	wasiWhenceSet = 0
	wasiWhenceCur = 1
	wasiWhenceEnd = 2
)

// WASI file types.
const (
	wasiFiletypeUnknown      = 0
	wasiFiletypeCharacter    = 2
	wasiFiletypeDirectory    = 3
	wasiFiletypeRegular      = 4
	wasiFiletypeSocketStream = 6
	wasiFiletypeSymbolicLink = 7
)

// Sizes of the WASI records the kernel writes.
const (
	wasiDirentHeaderSize = 24
	wasiFilestatSize     = 64
	wasiFdstatSize       = 24
	wasiIovecSize        = 8
)

// wasiDefaultCreatedPerm and wasiDefaultDirectoryPerm supply permissions for
// created files and directories, less the umask. WASI has no permission
// argument for either.
const (
	wasiDefaultCreatedPerm   = 0o666
	wasiDefaultDirectoryPerm = 0o777
)

// wasiRightsEverything and wasiRightsTerminal grant every right except that
// wasiRightsTerminal omits fd_seek and fd_tell, which wasi-libc's isatty
// requires a terminal to lack.
const (
	wasiRightsEverything = ^uint64(0)
	wasiRightsTerminal   = wasiRightsEverything &^ (1<<2 | 1<<5)
)

// wasiFSFunctions returns the filesystem functions of the
// wasi_snapshot_preview1 module.
func wasiFSFunctions() map[string]hostFunc {
	return map[string]hostFunc{
		"fd_advise": wasiFunc("iijji", func(_ context.Context, p *Process, _ engine.Memory, a []uint64) uint32 {
			return p.wasiFDCheck(uint32(a[0]))
		}),
		"fd_allocate": wasiFunc("iijj", func(_ context.Context, p *Process, _ engine.Memory, a []uint64) uint32 {
			return p.wasiFDAllocate(uint32(a[0]), a[1], a[2])
		}),
		"fd_close": wasiFunc("ii", func(_ context.Context, p *Process, _ engine.Memory, a []uint64) uint32 {
			return wasiErrnoFromResult(p.sysClose(uint32(a[0])))
		}),
		"fd_datasync": wasiFunc("ii", func(_ context.Context, p *Process, _ engine.Memory, a []uint64) uint32 {
			return wasiErrnoFromResult(p.sysFsync(uint32(a[0])))
		}),
		"fd_fdstat_get": wasiFunc("iii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiFDFdstatGet(mem, uint32(a[0]), uint32(a[1]))
		}),
		"fd_fdstat_set_flags": wasiFunc("iii", func(_ context.Context, p *Process, _ engine.Memory, a []uint64) uint32 {
			return p.wasiFDFdstatSetFlags(uint32(a[0]), uint32(a[1]))
		}),
		"fd_fdstat_set_rights": wasiFunc("iijj", func(_ context.Context, p *Process, _ engine.Memory, a []uint64) uint32 {
			return p.wasiFDCheck(uint32(a[0]))
		}),
		"fd_filestat_get": wasiFunc("iii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiFDFileStatGet(mem, uint32(a[0]), uint32(a[1]))
		}),
		"fd_filestat_set_size": wasiFunc("iij", func(_ context.Context, p *Process, _ engine.Memory, a []uint64) uint32 {
			return p.wasiFDFileStatSetSize(uint32(a[0]), a[1])
		}),
		"fd_filestat_set_times": wasiFunc("iijji", func(_ context.Context, p *Process, _ engine.Memory, a []uint64) uint32 {
			return p.wasiFDFileStatSetTimes(uint32(a[0]), a[1], a[2], uint32(a[3]))
		}),
		"fd_pread": wasiFunc("iiiiji", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiFDPread(mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), a[3], uint32(a[4]))
		}),
		// No descriptor is a preopened directory.
		"fd_prestat_dir_name": wasiFunc("iiii", func(context.Context, *Process, engine.Memory, []uint64) uint32 {
			return wasiErrnoBadf
		}),
		"fd_prestat_get": wasiFunc("iii", func(context.Context, *Process, engine.Memory, []uint64) uint32 {
			return wasiErrnoBadf
		}),
		"fd_pwrite": wasiFunc("iiiiji", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiFDPwrite(mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), a[3], uint32(a[4]))
		}),
		"fd_read": wasiFunc("iiiii", func(ctx context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiFDRead(ctx, mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]))
		}),
		"fd_readdir": wasiFunc("iiiiji", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiFDReadDir(mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), a[3], uint32(a[4]))
		}),
		"fd_renumber": wasiFunc("iii", func(_ context.Context, p *Process, _ engine.Memory, a []uint64) uint32 {
			return p.wasiFDRenumber(uint32(a[0]), uint32(a[1]))
		}),
		"fd_seek": wasiFunc("iijii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiFDSeek(mem, uint32(a[0]), int64(a[1]), uint32(a[2]), uint32(a[3]))
		}),
		"fd_sync": wasiFunc("ii", func(_ context.Context, p *Process, _ engine.Memory, a []uint64) uint32 {
			return wasiErrnoFromResult(p.sysFsync(uint32(a[0])))
		}),
		"fd_tell": wasiFunc("iii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiFDTell(mem, uint32(a[0]), uint32(a[1]))
		}),
		"fd_write": wasiFunc("iiiii", func(ctx context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiFDWrite(ctx, mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]))
		}),
		"path_create_directory": wasiFunc("iiii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiPathCreateDirectory(mem, uint32(a[0]), uint32(a[1]), uint32(a[2]))
		}),
		"path_filestat_get": wasiFunc("iiiiii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiPathFileStatGet(mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]), uint32(a[4]))
		}),
		"path_filestat_set_times": wasiFunc("iiiiijji", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiPathFileStatSetTimes(mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]), a[4], a[5], uint32(a[6]))
		}),
		"path_link": wasiFunc("iiiiiiii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiPathLink(mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]), uint32(a[4]), uint32(a[5]), uint32(a[6]))
		}),
		"path_open": wasiFunc("iiiiiijjii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiPathOpen(mem, uint32(a[0]), uint32(a[2]), uint32(a[3]), uint32(a[4]), uint32(a[7]), uint32(a[8]))
		}),
		"path_readlink": wasiFunc("iiiiiii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiPathReadlink(mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]), uint32(a[4]), uint32(a[5]))
		}),
		"path_remove_directory": wasiFunc("iiii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiPathRemoveDirectory(mem, uint32(a[0]), uint32(a[1]), uint32(a[2]))
		}),
		"path_rename": wasiFunc("iiiiiii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiPathRename(mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]), uint32(a[4]), uint32(a[5]))
		}),
		"path_symlink": wasiFunc("iiiiii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiPathSymlink(mem, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]), uint32(a[4]))
		}),
		"path_unlink_file": wasiFunc("iiii", func(_ context.Context, p *Process, mem engine.Memory, a []uint64) uint32 {
			return p.wasiPathUnlinkFile(mem, uint32(a[0]), uint32(a[1]), uint32(a[2]))
		}),
	}
}

// wasiFDCheck checks that fd is open, for the calls the kernel accepts
// without acting on them.
func (p *Process) wasiFDCheck(fd uint32) uint32 {
	if _, ok := p.fds[fd]; !ok {
		return wasiErrnoBadf
	}
	return wasiErrnoSuccess
}

// wasiFDNode returns the node a descriptor reports in its file status. WASI
// has no pipe file type, so a pipe reports as a character device that is not
// a terminal.
func (p *Process) wasiFDNode(fd uint32) (*VNode, uint32) {
	of, ok := p.fds[fd]
	switch {
	case !ok:
		return nil, wasiErrnoBadf
	case of.sock != nil:
		return of.sock.vnode(), wasiErrnoSuccess
	case of.node == nil:
		return &VNode{mode: sIFCHR | 0o600, nlink: 1}, wasiErrnoSuccess
	}
	return of.node, wasiErrnoSuccess
}

// wasiFDFdstatGet writes a descriptor's type, flags and rights to statPtr.
// wasiAtFDCWD reports the working directory, since wasi-libc's openat reads
// the rights a new file inherits from its directory descriptor.
func (p *Process) wasiFDFdstatGet(mem engine.Memory, fd, statPtr uint32) uint32 {
	// Find the node and its open flags.
	var node *VNode
	var openFlags uint32
	if fd == wasiAtFDCWD {
		node = p.kernel.vfs.lookup(p.cwd)
		if node == nil {
			return wasiErrno(ENOENT)
		}
	} else {
		var errno uint32
		node, errno = p.wasiFDNode(fd)
		if errno != wasiErrnoSuccess {
			return errno
		}
		openFlags = p.fds[fd].flags
	}

	// Work out its flags and rights. A terminal lacks the seek rights.
	var flags uint16
	if openFlags&oAPPEND != 0 {
		flags |= wasiFdflagsAppend
	}
	if openFlags&oNONBLOCK != 0 {
		flags |= wasiFdflagsNonblock
	}
	rights := wasiRightsEverything
	if p.fdTerminal(fd) != nil {
		rights = wasiRightsTerminal
	}

	// Write the record.
	var buf [wasiFdstatSize]byte
	buf[0] = wasiFiletype(node)
	binary.LittleEndian.PutUint16(buf[2:], flags)
	binary.LittleEndian.PutUint64(buf[8:], rights)
	binary.LittleEndian.PutUint64(buf[16:], rights)
	return wasiWrite(mem, statPtr, buf[:])
}

// wasiFDFdstatSetFlags sets or clears a descriptor's append and nonblocking
// flags. Setting append moves the offset to the end of the file.
func (p *Process) wasiFDFdstatSetFlags(fd, flags uint32) uint32 {
	// Set the nonblocking flag and clear append.
	of, ok := p.fds[fd]
	if !ok {
		return wasiErrnoBadf
	}
	of.flags &^= oAPPEND | oNONBLOCK
	if flags&wasiFdflagsNonblock != 0 {
		of.flags |= oNONBLOCK
	}
	if flags&wasiFdflagsAppend == 0 {
		return wasiErrnoSuccess
	}

	// Set append and move to the end.
	of.flags |= oAPPEND
	if of.node != nil {
		p.kernel.vfs.refreshHost(of.node)
		of.offset = of.node.size
	}
	return wasiErrnoSuccess
}

// wasiFDFileStatGet writes a descriptor's file status to statPtr.
func (p *Process) wasiFDFileStatGet(mem engine.Memory, fd, statPtr uint32) uint32 {
	node, errno := p.wasiFDNode(fd)
	if errno != wasiErrnoSuccess {
		return errno
	}
	return p.wasiWriteFileStat(mem, statPtr, node)
}

// wasiFDFileStatSetSize truncates or extends a descriptor's file to size.
func (p *Process) wasiFDFileStatSetSize(fd uint32, size uint64) uint32 {
	of, ok := p.fds[fd]
	switch {
	case size > math.MaxInt64:
		return wasiErrnoOverflow
	case !ok || of.node == nil:
		return wasiErrnoBadf
	}
	return wasiErrnoFromResult(p.truncateNode(of.node, int64(size)))
}

// wasiFDFileStatSetTimes sets a descriptor's file times.
func (p *Process) wasiFDFileStatSetTimes(fd uint32, atim, mtim uint64, flags uint32) uint32 {
	node, errno := p.wasiFDNode(fd)
	if errno != wasiErrnoSuccess {
		return errno
	}
	return p.wasiSetNodeTimes(node, atim, mtim, flags)
}

// wasiTransfer moves data through the iovecs at iovsPtr, calling move with
// each nonempty buffer, and writes the bytes moved to nPtr. A short move
// ends the transfer. A failed move returns its errno when nothing moved yet,
// and otherwise ends the transfer with the bytes already moved. move returns
// zero bytes when it fails.
func wasiTransfer(mem engine.Memory, iovsPtr, iovsLen, nPtr uint32, move func(base, length uint32) (uint32, uint32)) uint32 {
	total, errno := wasiMoveIovecs(mem, iovsPtr, iovsLen, move)
	switch {
	case errno != wasiErrnoSuccess:
		return errno
	case !mem.WriteUint32Le(nPtr, total):
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}

// wasiMoveIovecs calls move on each buffer of the iovecs at iovsPtr until one
// moves less than its length, and returns the total moved. An error after
// some bytes moved ends the transfer with that total.
func wasiMoveIovecs(mem engine.Memory, iovsPtr, iovsLen uint32, move func(base, length uint32) (uint32, uint32)) (uint32, uint32) {
	var total uint32
	for i := range iovsLen {
		base, ok1 := mem.ReadUint32Le(iovsPtr + i*wasiIovecSize)
		length, ok2 := mem.ReadUint32Le(iovsPtr + i*wasiIovecSize + 4)
		if !ok1 || !ok2 {
			return 0, wasiErrnoFault
		}
		if length == 0 {
			continue
		}
		n, errno := move(base, length)
		if errno != wasiErrnoSuccess && total == 0 {
			return 0, errno
		}
		total += n
		if n < length {
			break
		}
	}
	return total, wasiErrnoSuccess
}

// wasiResult splits a syscall result into a byte count and a WASI errno.
func wasiResult(res uint64) (uint32, uint32) {
	if errnoFailed(res) {
		return 0, wasiErrnoFromResult(res)
	}
	return uint32(res), wasiErrnoSuccess
}

// wasiFDRead reads a descriptor into the iovecs at iovsPtr.
func (p *Process) wasiFDRead(ctx context.Context, mem engine.Memory, fd, iovsPtr, iovsLen, nreadPtr uint32) uint32 {
	return wasiTransfer(mem, iovsPtr, iovsLen, nreadPtr, p.wasiReadMove(ctx, fd))
}

// wasiReadMove returns the move of a read of fd into iovecs. Only the first
// buffer's read waits, so a read that fills one buffer returns what follows
// it at once instead of waiting to fill the next buffer.
func (p *Process) wasiReadMove(ctx context.Context, fd uint32) func(base, length uint32) (uint32, uint32) {
	return func(base, length uint32) (uint32, uint32) {
		n, errno := wasiResult(p.sysRead(ctx, fd, base, length))
		ctx = nonblocking(ctx)
		return n, errno
	}
}

// wasiFDWrite writes the iovecs at iovsPtr to a descriptor.
func (p *Process) wasiFDWrite(ctx context.Context, mem engine.Memory, fd, iovsPtr, iovsLen, nwrittenPtr uint32) uint32 {
	return wasiTransfer(mem, iovsPtr, iovsLen, nwrittenPtr, func(base, length uint32) (uint32, uint32) {
		return wasiResult(p.sysWrite(ctx, fd, base, length))
	})
}

// wasiFDPread reads a file at offset into the iovecs at iovsPtr without
// moving the descriptor's offset.
func (p *Process) wasiFDPread(mem engine.Memory, fd, iovsPtr, iovsLen uint32, offset uint64, nreadPtr uint32) uint32 {
	// Find the file.
	if offset > math.MaxInt64 {
		return wasiErrnoOverflow
	}
	of, errno := p.wasiSeekableFD(fd)
	if errno != wasiErrnoSuccess {
		return errno
	}

	// Read each buffer from where the last ended.
	pos := int64(offset)
	return wasiTransfer(mem, iovsPtr, iovsLen, nreadPtr, func(base, length uint32) (uint32, uint32) {
		// Read the file into the buffer.
		buf, res := p.kernel.readNodeBytes(of, pos, int(length))
		if res != 0 {
			return 0, wasiErrnoFromResult(res)
		}
		if len(buf) != 0 && !mem.Write(base, buf) {
			return 0, wasiErrnoFault
		}
		pos += int64(len(buf))
		return uint32(len(buf)), wasiErrnoSuccess
	})
}

// wasiFDPwrite writes the iovecs at iovsPtr to a file at offset without
// moving the descriptor's offset.
func (p *Process) wasiFDPwrite(mem engine.Memory, fd, iovsPtr, iovsLen uint32, offset uint64, nwrittenPtr uint32) uint32 {
	// Find the file.
	if offset > math.MaxInt64 {
		return wasiErrnoOverflow
	}
	of, errno := p.wasiSeekableFD(fd)
	if errno != wasiErrnoSuccess {
		return errno
	}

	// Write each buffer after the last buffer.
	pos := int64(offset)
	return wasiTransfer(mem, iovsPtr, iovsLen, nwrittenPtr, func(base, length uint32) (uint32, uint32) {
		// Write the buffer to the file if the seals allow it.
		buf, ok := mem.Read(base, length)
		if !ok {
			return 0, wasiErrnoFault
		}
		if res := of.node.checkWrite(pos, len(buf)); res != 0 {
			return 0, wasiErrnoFromResult(res)
		}
		n, res := p.kernel.writeNodeBytes(of, pos, buf)
		if res != 0 {
			return 0, wasiErrnoFromResult(res)
		}
		pos += int64(n)
		return uint32(n), wasiErrnoSuccess
	})
}

// wasiFDReadDir writes the entries of a directory from cookie on to the
// buffer at bufPtr, as many as fit whole, and the bytes written to usedPtr.
// Each entry's cookie is the index of the entry after it. Reading from
// cookie 0 takes a fresh listing of the directory. Subsequent reads continue
// in the listing, skipping entries removed since.
func (p *Process) wasiFDReadDir(mem engine.Memory, fd, bufPtr, bufLen uint32, cookie uint64, usedPtr uint32) uint32 {
	// List the directory.
	of, ok := p.fds[fd]
	if !ok {
		return wasiErrnoBadf
	}
	node := of.node
	if node == nil || node.mode&sIFMT != sIFDIR {
		return wasiErrno(ENOTDIR)
	}
	if of.dirNames == nil || cookie == 0 {
		p.kernel.vfs.refreshDirChildren(node)
		of.dirNames = slices.Sorted(maps.Keys(node.children))
	}

	// Write the entries that fit.
	var used uint32
	for idx := int(min(cookie, uint64(len(of.dirNames)))); idx < len(of.dirNames); idx++ {
		name := of.dirNames[idx]
		child := node.children[name]
		size := uint32(wasiDirentHeaderSize + len(name))
		if child == nil {
			continue
		}
		if used+size > bufLen {
			break
		}
		if errno := wasiWriteDirent(mem, bufPtr+used, uint64(idx+1), child, name); errno != wasiErrnoSuccess {
			return errno
		}
		used += size
	}
	if !mem.WriteUint32Le(usedPtr, used) {
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}

// wasiFDRenumber moves descriptor from, with its close-on-exec flag, to
// number to, closing the file to held before.
func (p *Process) wasiFDRenumber(from, to uint32) uint32 {
	of, ok := p.fds[from]
	if !ok {
		return wasiErrnoBadf
	}
	if from != to {
		_, cloexec := p.cloexec[from]
		p.setFD(to, of)
		p.setCloexec(to, cloexec)
		_ = p.sysClose(from)
	}
	return wasiErrnoSuccess
}

// wasiSeekableFD returns the open file of a descriptor that has an offset.
// Pipes and terminals do not.
func (p *Process) wasiSeekableFD(fd uint32) (*OpenFile, uint32) {
	of, ok := p.fds[fd]
	switch {
	case !ok:
		return nil, wasiErrnoBadf
	case of.node == nil || of.tty != nil || of.ptm != nil:
		return nil, wasiErrno(ESPIPE)
	}
	return of, wasiErrnoSuccess
}

// wasiFDSeek moves a descriptor's offset and writes the new offset to
// newOffsetPtr. An offset before the start of the file fails with EINVAL.
func (p *Process) wasiFDSeek(mem engine.Memory, fd uint32, offset int64, whence, newOffsetPtr uint32) uint32 {
	// Find the file.
	of, errno := p.wasiSeekableFD(fd)
	if errno != wasiErrnoSuccess {
		return errno
	}

	// Work out the new offset from whence.
	switch whence {
	case wasiWhenceSet:
	case wasiWhenceCur:
		offset += of.offset
	case wasiWhenceEnd:
		p.kernel.vfs.refreshHost(of.node)
		offset += of.node.size
	default:
		return wasiErrnoInval
	}
	if offset < 0 {
		return wasiErrnoInval
	}

	// Move to it.
	of.offset = offset
	if !mem.WriteUint64Le(newOffsetPtr, uint64(offset)) {
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}

// wasiFDTell writes a descriptor's offset to offsetPtr.
func (p *Process) wasiFDTell(mem engine.Memory, fd, offsetPtr uint32) uint32 {
	of, errno := p.wasiSeekableFD(fd)
	switch {
	case errno != wasiErrnoSuccess:
		return errno
	case !mem.WriteUint64Le(offsetPtr, uint64(of.offset)):
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}

// wasiFDAllocate extends a regular file to cover length bytes at offset.
func (p *Process) wasiFDAllocate(fd uint32, offset, length uint64) uint32 {
	// Find the file.
	if offset > math.MaxInt64 || length > math.MaxInt64-offset {
		return wasiErrnoOverflow
	}
	of, ok := p.fds[fd]
	if !ok || of.node == nil {
		return wasiErrnoBadf
	}
	if of.node.mode&sIFMT != sIFREG {
		return wasiErrnoInval
	}

	// Extend it when it is shorter.
	end := int64(offset + length)
	if of.node.size >= end {
		return wasiErrnoSuccess
	}
	return wasiErrnoFromResult(p.truncateNode(of.node, end))
}

// wasiOpenFlags pairs each WASI open flag with the open(2) flag it maps to.
var wasiOpenFlags = [...]struct{ wasi, posix uint32 }{
	{wasiOflagsCreat, oCREAT},
	{wasiOflagsDirectory, oDIRECTORY},
	{wasiOflagsExcl, oEXCL},
	{wasiOflagsTrunc, oTRUNC},
}

// wasiPathOpen opens a path read-write and writes the new descriptor to
// openedPtr. The kernel does not enforce rights, so it ignores them.
func (p *Process) wasiPathOpen(mem engine.Memory, dirfd, pathPtr, pathLen, oflags, fdflags, openedPtr uint32) uint32 {
	// Resolve the path and translate the flags.
	resolved, errno := p.wasiPathFromMemory(mem, dirfd, pathPtr, pathLen)
	if errno != wasiErrnoSuccess {
		return errno
	}
	flags := uint32(oRDWR)
	for _, f := range wasiOpenFlags {
		if oflags&f.wasi != 0 {
			flags |= f.posix
		}
	}
	if fdflags&wasiFdflagsAppend != 0 {
		flags |= oAPPEND
	}
	if fdflags&wasiFdflagsNonblock != 0 {
		flags |= oNONBLOCK
	}
	if fdflags&wasiFdflagsNoctty != 0 {
		flags |= oNOCTTY
	}
	if fdflags&wasiFdflagsCloexec != 0 {
		flags |= oCLOEXEC
	}

	// Open the file and report its descriptor.
	fd, errno := wasiResult(p.openResolved(resolved, flags, wasiDefaultCreatedPerm&^p.umask))
	if errno != wasiErrnoSuccess {
		return errno
	}
	if !mem.WriteUint32Le(openedPtr, fd) {
		_ = p.sysClose(fd)
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}

// wasiPathFileStatGet writes the file status of a path to statPtr.
func (p *Process) wasiPathFileStatGet(mem engine.Memory, dirfd, flags, pathPtr, pathLen, statPtr uint32) uint32 {
	node, errno := p.wasiLookupPath(mem, dirfd, flags, pathPtr, pathLen)
	if errno != wasiErrnoSuccess {
		return errno
	}
	return p.wasiWriteFileStat(mem, statPtr, node)
}

// wasiPathFileStatSetTimes sets the file times of a path.
func (p *Process) wasiPathFileStatSetTimes(mem engine.Memory, dirfd, flags, pathPtr, pathLen uint32, atim, mtim uint64, fstflags uint32) uint32 {
	node, errno := p.wasiLookupPath(mem, dirfd, flags, pathPtr, pathLen)
	if errno != wasiErrnoSuccess {
		return errno
	}
	return p.wasiSetNodeTimes(node, atim, mtim, fstflags)
}

// wasiPathCreateDirectory creates a directory.
func (p *Process) wasiPathCreateDirectory(mem engine.Memory, dirfd, pathPtr, pathLen uint32) uint32 {
	resolved, errno := p.wasiPathFromMemory(mem, dirfd, pathPtr, pathLen)
	if errno != wasiErrnoSuccess {
		return errno
	}
	return wasiErrnoFromResult(p.mkdirResolved(resolved, wasiDefaultDirectoryPerm&^p.umask))
}

// wasiPathRemoveDirectory removes an empty directory.
func (p *Process) wasiPathRemoveDirectory(mem engine.Memory, dirfd, pathPtr, pathLen uint32) uint32 {
	resolved, errno := p.wasiPathFromMemory(mem, dirfd, pathPtr, pathLen)
	if errno != wasiErrnoSuccess {
		return errno
	}
	return wasiErrnoFromResult(p.unlinkResolved(resolved, true))
}

// wasiPathUnlinkFile removes a file that is not a directory.
func (p *Process) wasiPathUnlinkFile(mem engine.Memory, dirfd, pathPtr, pathLen uint32) uint32 {
	resolved, errno := p.wasiPathFromMemory(mem, dirfd, pathPtr, pathLen)
	if errno != wasiErrnoSuccess {
		return errno
	}
	return wasiErrnoFromResult(p.unlinkResolved(resolved, false))
}

// wasiPathRename moves a path to another.
func (p *Process) wasiPathRename(mem engine.Memory, olddirfd, oldpathPtr, oldpathLen, newdirfd, newpathPtr, newpathLen uint32) uint32 {
	oldResolved, newResolved, errno := p.wasiPathPair(mem, olddirfd, oldpathPtr, oldpathLen, newdirfd, newpathPtr, newpathLen)
	if errno != wasiErrnoSuccess {
		return errno
	}
	return wasiErrnoFromResult(p.renameResolved(oldResolved, newResolved))
}

// wasiPathLink creates a hard link at the new path to the old one.
func (p *Process) wasiPathLink(mem engine.Memory, olddirfd, oldflags, oldpathPtr, oldpathLen, newdirfd, newpathPtr, newpathLen uint32) uint32 {
	oldResolved, newResolved, errno := p.wasiPathPair(mem, olddirfd, oldpathPtr, oldpathLen, newdirfd, newpathPtr, newpathLen)
	if errno != wasiErrnoSuccess {
		return errno
	}
	return wasiErrnoFromResult(p.linkResolved(oldResolved, newResolved, oldflags&wasiLookupSymlinkFollow != 0))
}

// wasiPathSymlink creates a symlink at the new path pointing to the old one.
func (p *Process) wasiPathSymlink(mem engine.Memory, oldpathPtr, oldpathLen, dirfd, newpathPtr, newpathLen uint32) uint32 {
	// Read the target as it is and resolve the link's path.
	target, errno := wasiReadPath(mem, oldpathPtr, oldpathLen)
	if errno != wasiErrnoSuccess {
		return errno
	}
	newResolved, errno := p.wasiPathFromMemory(mem, dirfd, newpathPtr, newpathLen)
	if errno != wasiErrnoSuccess {
		return errno
	}

	// Create the link.
	return wasiErrnoFromResult(p.symlinkResolved(target, newResolved))
}

// wasiPathReadlink writes a symlink's target, cut to bufLen bytes, to bufPtr
// and its length to usedPtr.
func (p *Process) wasiPathReadlink(mem engine.Memory, dirfd, pathPtr, pathLen, bufPtr, bufLen, usedPtr uint32) uint32 {
	// Find the target.
	resolved, errno := p.wasiPathFromMemory(mem, dirfd, pathPtr, pathLen)
	if errno != wasiErrnoSuccess {
		return errno
	}
	node := p.kernel.vfs.lookupNoFollow(resolved)
	var target string
	switch {
	case node == nil:
		return wasiErrno(ENOENT)
	case node.mode&sIFMT != sIFLNK:
		return wasiErrnoInval
	default:
		target = node.symlink
	}

	// Write it.
	target = target[:min(len(target), int(bufLen))]
	if !mem.Write(bufPtr, []byte(target)) || !mem.WriteUint32Le(usedPtr, uint32(len(target))) {
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}

// wasiPathPair reads and resolves the two paths of a call that takes an old
// path and a new one.
func (p *Process) wasiPathPair(mem engine.Memory, olddirfd, oldPtr, oldLen, newdirfd, newPtr, newLen uint32) (string, string, uint32) {
	oldResolved, errno := p.wasiPathFromMemory(mem, olddirfd, oldPtr, oldLen)
	if errno != wasiErrnoSuccess {
		return "", "", errno
	}
	newResolved, errno := p.wasiPathFromMemory(mem, newdirfd, newPtr, newLen)
	return oldResolved, newResolved, errno
}

// wasiPathFromMemory reads a path and resolves it against dirfd.
func (p *Process) wasiPathFromMemory(mem engine.Memory, dirfd, pathPtr, pathLen uint32) (string, uint32) {
	path, errno := wasiReadPath(mem, pathPtr, pathLen)
	if errno != wasiErrnoSuccess {
		return "", errno
	}
	return p.wasiResolvePath(dirfd, path)
}

// wasiResolvePath resolves path against the directory of dirfd, or against
// the working directory when dirfd is wasiAtFDCWD.
func (p *Process) wasiResolvePath(dirfd uint32, path string) (string, uint32) {
	// Take an absolute path as it is.
	if path == "" {
		return "", wasiErrno(ENOENT)
	}
	if path[0] == '/' {
		return cleanPath(path), wasiErrnoSuccess
	}

	// Join a relative path to its directory.
	if dirfd == wasiAtFDCWD {
		return cleanPath(p.cwd + "/" + path), wasiErrnoSuccess
	}
	of, ok := p.fds[dirfd]
	switch {
	case !ok || of.path == "":
		return "", wasiErrnoBadf
	case of.node == nil || of.node.mode&sIFMT != sIFDIR:
		return "", wasiErrno(ENOTDIR)
	}
	return cleanPath(of.path + "/" + path), wasiErrnoSuccess
}

// wasiLookupPath returns the node at a path, following a final symlink when
// flags ask for it.
func (p *Process) wasiLookupPath(mem engine.Memory, dirfd, flags, pathPtr, pathLen uint32) (*VNode, uint32) {
	// Resolve the path.
	resolved, errno := p.wasiPathFromMemory(mem, dirfd, pathPtr, pathLen)
	if errno != wasiErrnoSuccess {
		return nil, errno
	}

	// Find its node.
	node := p.kernel.vfs.lookupNoFollow(resolved)
	if flags&wasiLookupSymlinkFollow != 0 {
		node = p.kernel.vfs.lookup(resolved)
	}
	if node == nil {
		return nil, wasiErrno(ENOENT)
	}
	return node, wasiErrnoSuccess
}

// wasiSetNodeTimes sets a node's access and modification times as fstflags
// directs, and its change time to now. A host-backed file's times change on
// the root filesystem too.
func (p *Process) wasiSetNodeTimes(node *VNode, atim, mtim uint64, fstflags uint32) uint32 {
	// Work out the new times.
	now := time.Now()
	atime, ok1 := wasiSetTime(fstflags, wasiFilestatSetAtim, wasiFilestatSetAtimNow, atim, now, node.atime)
	mtime, ok2 := wasiSetTime(fstflags, wasiFilestatSetMtim, wasiFilestatSetMtimNow, mtim, now, node.mtime)
	if !ok1 || !ok2 {
		return wasiErrnoInval
	}

	// Set them on the root filesystem, then the node.
	if node.hostPath != "" && node.mode&sIFMT != sIFLNK {
		if err := p.kernel.vfs.chtimesHost(node.hostPath, cmp.Or(atime, now), cmp.Or(mtime, now)); err != nil {
			return wasiErrnoIo
		}
	}
	node.atime, node.mtime, node.ctime = atime, mtime, now
	return wasiErrnoSuccess
}

// wasiSetTime returns the time that the fstflags bits set and setNow choose:
// t for set, now for setNow and cur for neither. Both together are invalid.
func wasiSetTime(fstflags, set, setNow uint32, t uint64, now, cur time.Time) (time.Time, bool) {
	switch fstflags & (set | setNow) {
	case 0:
		return cur, true
	case set:
		return time.Unix(0, int64(t)), true
	case setNow:
		return now, true
	}
	return cur, false
}

// wasiWriteFileStat writes a node's file status to ptr. A node without a
// time reports the current time.
func (p *Process) wasiWriteFileStat(mem engine.Memory, ptr uint32, node *VNode) uint32 {
	// Bring a host-backed node up to date.
	p.kernel.vfs.refreshHost(node)

	// Write the device, inode, type, link count and size.
	var buf [wasiFilestatSize]byte
	binary.LittleEndian.PutUint64(buf[0:], 1)
	binary.LittleEndian.PutUint64(buf[8:], node.ino)
	buf[16] = wasiFiletype(node)
	binary.LittleEndian.PutUint64(buf[24:], uint64(max(node.nlink, 1)))
	binary.LittleEndian.PutUint64(buf[32:], uint64(node.size))

	// Write the times.
	now := time.Now()
	binary.LittleEndian.PutUint64(buf[40:], uint64(cmp.Or(node.atime, now).UnixNano()))
	binary.LittleEndian.PutUint64(buf[48:], uint64(cmp.Or(node.mtime, now).UnixNano()))
	binary.LittleEndian.PutUint64(buf[56:], uint64(cmp.Or(node.ctime, now).UnixNano()))
	return wasiWrite(mem, ptr, buf[:])
}

// wasiWriteDirent writes a directory entry for node under name to ptr.
func wasiWriteDirent(mem engine.Memory, ptr uint32, nextCookie uint64, node *VNode, name string) uint32 {
	// Write the header, then the name.
	var buf [wasiDirentHeaderSize]byte
	binary.LittleEndian.PutUint64(buf[0:], nextCookie)
	binary.LittleEndian.PutUint64(buf[8:], node.ino)
	binary.LittleEndian.PutUint32(buf[16:], uint32(len(name)))
	buf[20] = wasiFiletype(node)
	return wasiWrite(mem, ptr, append(buf[:], name...))
}

// wasiFiletype returns the WASI file type of a node.
func wasiFiletype(node *VNode) uint8 {
	switch node.mode & sIFMT {
	case sIFCHR:
		return wasiFiletypeCharacter
	case sIFDIR:
		return wasiFiletypeDirectory
	case sIFREG:
		return wasiFiletypeRegular
	case sIFLNK:
		return wasiFiletypeSymbolicLink
	case sIFSOCK:
		return wasiFiletypeSocketStream
	}
	return wasiFiletypeUnknown
}

// wasiWrite writes buf to ptr.
func wasiWrite(mem engine.Memory, ptr uint32, buf []byte) uint32 {
	if !mem.Write(ptr, buf) {
		return wasiErrnoFault
	}
	return wasiErrnoSuccess
}
