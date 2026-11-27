package kernel

import (
	"cmp"
	"context"
	"encoding/binary"
	"os"
	"time"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// statSize is the size of wasi-libc's struct stat on wasm32.
const statSize = 144

// sysSendfile copies up to count bytes from descriptor inFd to outFd. When
// offsetPtr is set, the copy reads the file at the offset stored there and
// stores the new offset back, leaving inFd's offset alone. Otherwise it reads
// inFd as read does, so a pipe or terminal source works as on Linux.
func (p *Process) sysSendfile(ctx context.Context, outFd, inFd, offsetPtr, count uint32) uint64 {
	// Find the source.
	in, ok := p.fds[inFd]
	if !ok {
		return negErrno(EBADF)
	}
	if in.node != nil && in.node.mode&sIFMT == sIFDIR {
		return negErrno(EINVAL)
	}

	// Let a caught signal interrupt a blocking read or write.
	ctx, release := p.interruptible(ctx)
	defer release()

	// Read at the given offset, or from the descriptor.
	mem := p.inst.Memory()
	var buf []byte
	var off int64
	var errno uint64
	if offsetPtr != 0 {
		if in.node == nil || in.node.mode&sIFMT != sIFREG {
			return negErrno(ESPIPE)
		}
		v, ok := mem.ReadUint64Le(offsetPtr)
		if !ok {
			return negErrno(EFAULT)
		}
		off = int64(v)
		buf, errno = p.kernel.readNodeBytes(in, off, int(count))
	} else {
		buf, errno = p.readFile(ctx, in, int(count))
	}
	if errno != 0 || len(buf) == 0 {
		return errno
	}

	// Copy the bytes. The file's offset ends after the bytes written, so a
	// short or failed write returns the rest of a regular file's read.
	res := p.writeOutBytes(ctx, outFd, buf)
	var written int64
	if !errnoFailed(res) {
		written = int64(res)
	}
	switch {
	case offsetPtr != 0:
		mem.WriteUint64Le(offsetPtr, uint64(off+written))
	case in.node != nil && in.node.mode&sIFMT == sIFREG:
		in.offset -= int64(len(buf)) - written
	}
	return res
}

// sysFstat writes the status of descriptor fd to statPtr.
func (p *Process) sysFstat(fd, statPtr uint32) uint64 {
	node, errno := p.fdNode(fd)
	if errno != 0 {
		return errno
	}
	return p.writeStat(p.inst.Memory(), statPtr, node)
}

// sysFstatat writes the status of the file at path, relative to dirfd, to
// statPtr. It replaces wasi-libc's __wasilibc_nocwd_fstatat, so dirfd and
// path name the file as WASI path calls do. AT_SYMLINK_NOFOLLOW, in its Linux
// or wasi-libc value, reports a final symlink itself.
func (p *Process) sysFstatat(dirfd, pathPtr, statPtr, flags uint32) uint64 {
	// Resolve the path as the WASI path calls do.
	mem := p.inst.Memory()
	resolved, werr := p.wasiResolvePath(dirfd, readCString(mem, pathPtr))
	if werr != wasiErrnoSuccess {
		return negErrno(linuxErrno(werr))
	}

	// Write the status of its node.
	if flags&wasiAtSymlinkNofollow != 0 {
		flags |= atSymlinkNofollow
	}
	node, errno := p.lookup(resolved, flags)
	if errno != 0 {
		return errno
	}
	return p.writeStat(mem, statPtr, node)
}

// sysFchmodat sets the permission bits of the file at path, relative to
// dirfd. Linux cannot change a symlink's mode, so AT_SYMLINK_NOFOLLOW on one
// fails.
func (p *Process) sysFchmodat(dirfd, pathPtr, mode, flags uint32) uint64 {
	// Resolve the path.
	path := readCString(p.inst.Memory(), pathPtr)
	if path == "" {
		return negErrno(ENOENT)
	}

	// Set the mode of its node.
	node, errno := p.lookup(resolvePath(p.cwd, p.fds, dirfd, path), flags)
	if errno != 0 {
		return errno
	}
	return p.chmodNode(node, mode)
}

// sysFchmod sets the permission bits of descriptor fd's file.
func (p *Process) sysFchmod(fd, mode uint32) uint64 {
	node, errno := p.fdNode(fd)
	if errno != 0 {
		return errno
	}
	return p.chmodNode(node, mode)
}

// fdNode returns the node of descriptor fd. A pipe or socket has no node,
// so it gets one that describes a FIFO or socket with its inode number. An
// event, timer or epoll file gets one with no type, as Linux reports an
// anonymous inode.
func (p *Process) fdNode(fd uint32) (*VNode, uint64) {
	of, ok := p.fds[fd]
	switch {
	case !ok:
		return nil, negErrno(EBADF)
	case of.sock != nil:
		return of.sock.vnode(), 0
	case of.pipe() != nil:
		return &VNode{ino: of.pipe().ino, mode: sIFIFO | 0o600, nlink: 1}, 0
	case of.node == nil:
		return &VNode{mode: 0o600, nlink: 1}, 0
	}
	return of.node, 0
}

// lookup finds the node at a resolved path, following a final symlink unless
// flags holds AT_SYMLINK_NOFOLLOW.
func (p *Process) lookup(resolved string, flags uint32) (*VNode, uint64) {
	var node *VNode
	if flags&atSymlinkNofollow != 0 {
		node = p.kernel.vfs.lookupNoFollow(resolved)
	} else {
		node = p.kernel.vfs.lookup(resolved)
	}
	if node == nil {
		return nil, negErrno(ENOENT)
	}
	return node, 0
}

// chmodNode sets a node's permission bits, on the root filesystem too for a
// host-backed file. A file sealed with sealExec keeps its execute bits.
func (p *Process) chmodNode(node *VNode, mode uint32) uint64 {
	// Refuse a symlink and a sealed change.
	mode &= 0o7777
	switch {
	case node.mode&sIFMT == sIFLNK:
		return negErrno(EOPNOTSUPP)
	case node.seals&sealExec != 0 && (node.mode^mode)&0o111 != 0:
		return negErrno(EPERM)
	}

	// Set the mode on the root filesystem, then the node.
	if node.hostPath != "" {
		if err := p.kernel.vfs.chmodHost(node.hostPath, mode); err != nil {
			return negErrno(EIO)
		}
	}
	node.mode = node.mode&sIFMT | mode
	node.ctime = time.Now()
	return 0
}

// writeStat writes a node's status to ptr as wasi-libc's struct stat. Every
// file belongs to root, and a node without a time reports the current time.
func (p *Process) writeStat(mem engine.Memory, ptr uint32, node *VNode) uint64 {
	// Bring a host-backed node up to date.
	p.kernel.vfs.refreshHost(node)

	// Write the device, inode, link count, mode, device number and size.
	var buf [statSize]byte
	le := binary.LittleEndian
	le.PutUint64(buf[0:], 1)
	le.PutUint64(buf[8:], node.ino)
	le.PutUint64(buf[16:], uint64(max(node.nlink, 1)))
	le.PutUint32(buf[24:], node.mode)
	le.PutUint64(buf[40:], node.rdev)
	le.PutUint64(buf[48:], uint64(node.size))

	// Write the block size and the 512-byte blocks the file holds.
	le.PutUint32(buf[56:], 4096)
	le.PutUint64(buf[64:], uint64((node.size+511)/512))

	// Write the access, modification and change times.
	now := time.Now()
	for i, t := range []time.Time{node.atime, node.mtime, node.ctime} {
		t = cmp.Or(t, now)
		le.PutUint64(buf[72+16*i:], uint64(t.Unix()))
		le.PutUint32(buf[80+16*i:], uint32(t.Nanosecond()))
	}
	if !mem.Write(ptr, buf[:]) {
		return negErrno(EFAULT)
	}
	return 0
}

// sysMknodat creates a regular file, FIFO or device node. A mode without a
// file type creates a regular file.
func (p *Process) sysMknodat(dirfd, pathPtr, mode, dev uint32) uint64 {
	// Find the directory that will hold the node.
	path := readCString(p.inst.Memory(), pathPtr)
	if path == "" {
		return negErrno(ENOENT)
	}
	parent, name, errno := p.newEntry(resolvePath(p.cwd, p.fds, dirfd, path))
	if errno != 0 {
		return errno
	}

	// Create the node.
	switch typ := mode & sIFMT; typ {
	case 0, sIFREG:
		p.kernel.vfs.mkFile(parent, name, nil, mode&0o7777)
	case sIFIFO, sIFCHR, sIFBLK:
		p.kernel.vfs.mkNode(parent, name, typ|mode&0o7777, uint64(dev))
	default:
		return negErrno(EINVAL)
	}
	return 0
}

// sysFlock applies or releases an advisory lock. The kernel does not enforce
// advisory locks, so every valid request succeeds.
func (p *Process) sysFlock(fd, operation uint32) uint64 {
	if _, ok := p.fds[fd]; !ok {
		return negErrno(EBADF)
	}
	switch operation &^ lockNB {
	case lockSH, lockEX, lockUN:
		return 0
	}
	return negErrno(EINVAL)
}

// newEntry returns the directory that will hold a new entry at a resolved
// path, with the entry's name. It fails with ENOENT when the directory is
// missing and EEXIST when the entry exists.
func (p *Process) newEntry(resolved string) (*VNode, string, uint64) {
	// Find the directory.
	vfs := p.kernel.vfs
	dir, name := splitPath(resolved)
	parent := vfs.lookup(dir)
	if parent == nil || parent.mode&sIFMT != sIFDIR {
		return nil, "", negErrno(ENOENT)
	}

	// Check the name is free, including on the root filesystem.
	vfs.refreshDirChildren(parent)
	if parent.children[name] != nil {
		return nil, "", negErrno(EEXIST)
	}
	return parent, name, 0
}

// mkdirResolved creates a directory at a resolved path. A directory in a
// host-backed directory is created on the root filesystem.
func (p *Process) mkdirResolved(resolved string, mode uint32) uint64 {
	// Find the parent, and create an in-memory directory under an in-memory
	// parent.
	vfs := p.kernel.vfs
	parent, name, errno := p.newEntry(resolved)
	if errno != 0 {
		return errno
	}
	if parent.hostPath == "" {
		vfs.mkDir(parent, name)
		return 0
	}

	// Create the directory on the root filesystem and load its node.
	hostPath := hostChildPath(parent.hostPath, name)
	if err := vfs.host.MkdirAll(hostPath, os.FileMode(mode&0o7777)); err != nil {
		return negErrno(EIO)
	}
	vfs.child(parent, name)
	return 0
}

// unlinkResolved removes the entry at a resolved path. removeDir removes an
// empty directory, as rmdir does. Otherwise the entry must not be a
// directory, as unlink requires.
func (p *Process) unlinkResolved(resolved string, removeDir bool) uint64 {
	// Find the entry.
	vfs := p.kernel.vfs
	dir, name := splitPath(resolved)
	parent := vfs.lookup(dir)
	if parent == nil {
		return negErrno(ENOENT)
	}
	vfs.refreshDirChildren(parent)
	child := parent.children[name]
	if child == nil {
		return negErrno(ENOENT)
	}

	// Check its type against the call.
	isDir := child.mode&sIFMT == sIFDIR
	switch {
	case removeDir && !isDir:
		return negErrno(ENOTDIR)
	case !removeDir && isDir:
		return negErrno(EISDIR)
	case removeDir:
		vfs.refreshDirChildren(child)
		if len(child.children) != 0 {
			return negErrno(ENOTEMPTY)
		}
	}

	// Remove it from the root filesystem and the tree.
	if child.hostPath != "" {
		if err := vfs.host.Remove(child.hostPath); err != nil {
			return negErrno(EIO)
		}
	}
	delete(parent.children, name)
	return 0
}

// sysChdir changes the current working directory.
func (p *Process) sysChdir(pathPtr uint32) uint64 {
	path := readCString(p.inst.Memory(), pathPtr)
	return p.chdirResolved(resolvePath(p.cwd, p.fds, atFDCWD, path))
}

// sysGetcwd writes the working directory and its terminating NUL to buf and
// returns their length, or ERANGE when size cannot hold them.
func (p *Process) sysGetcwd(bufPtr, size uint32) uint64 {
	cwd := p.cwd + "\x00"
	if uint32(len(cwd)) > size {
		return negErrno(ERANGE)
	}
	p.inst.Memory().Write(bufPtr, []byte(cwd))
	return uint64(len(cwd))
}

// chdirResolved changes the current working directory to a resolved path.
func (p *Process) chdirResolved(resolved string) uint64 {
	node := p.kernel.vfs.lookup(resolved)
	switch {
	case node == nil:
		return negErrno(ENOENT)
	case node.mode&sIFMT != sIFDIR:
		return negErrno(ENOTDIR)
	}
	p.cwd = resolved
	return 0
}

// renameResolved moves the entry at a resolved path to another, renaming it
// on the root filesystem when both are host-backed.
func (p *Process) renameResolved(oldResolved, newResolved string) uint64 {
	// Find the entry.
	vfs := p.kernel.vfs
	oldDir, oldName := splitPath(oldResolved)
	oldParent := vfs.lookup(oldDir)
	if oldParent == nil {
		return negErrno(ENOENT)
	}
	vfs.refreshDirChildren(oldParent)
	child := oldParent.children[oldName]
	if child == nil {
		return negErrno(ENOENT)
	}

	// Find the destination directory.
	newDir, newName := splitPath(newResolved)
	newParent := vfs.lookup(newDir)
	switch {
	case newParent == nil:
		return negErrno(ENOENT)
	case newParent.mode&sIFMT != sIFDIR:
		return negErrno(ENOTDIR)
	}

	// Move the file on the root filesystem.
	if child.hostPath != "" && newParent.hostPath != "" {
		newHostPath := hostChildPath(newParent.hostPath, newName)
		if err := vfs.host.Rename(child.hostPath, newHostPath); err != nil {
			return negErrno(EIO)
		}
		child.hostPath = newHostPath
	}

	// Move the node in the tree.
	delete(oldParent.children, oldName)
	newParent.children[newName] = child
	child.parent = newParent
	return 0
}

// symlinkResolved creates an in-memory symbolic link to target at a resolved
// path.
func (p *Process) symlinkResolved(target, resolved string) uint64 {
	parent, name, errno := p.newEntry(resolved)
	if errno != 0 {
		return errno
	}
	p.kernel.vfs.mkSymlink(parent, name, target)
	return 0
}

// linkResolved creates a hard link at newResolved to the node at oldResolved.
// follow links the target of a symlink at oldResolved rather than the
// symlink.
func (p *Process) linkResolved(oldResolved, newResolved string, follow bool) uint64 {
	// Find the node to link.
	vfs := p.kernel.vfs
	node := vfs.lookupNoFollow(oldResolved)
	if follow {
		node = vfs.lookup(oldResolved)
	}
	switch {
	case node == nil:
		return negErrno(ENOENT)
	case node.mode&sIFMT == sIFDIR:
		return negErrno(EPERM)
	}

	// Add it under the new name.
	parent, name, errno := p.newEntry(newResolved)
	if errno != 0 {
		return errno
	}
	parent.children[name] = node
	node.nlink++
	return 0
}
