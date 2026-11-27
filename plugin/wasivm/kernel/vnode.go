package kernel

import "time"

// VNode is a file, directory, symlink or device in the VFS. A host-backed
// node mirrors the file at hostPath in the root filesystem, which holds its
// content. A host-backed directory loads its entries on demand.
type VNode struct {
	// ino is the inode number.
	ino uint64
	// mode holds the file type and permission bits.
	mode  uint32
	nlink uint32
	// rdev is the device number of a device node.
	rdev uint64
	size int64
	// data is an in-memory regular file's content.
	data []byte
	// children holds a directory's entries by name.
	children map[string]*VNode
	// xattrs holds the extended attributes by full name.
	xattrs map[string][]byte
	parent *VNode
	// symlink is a symlink's target.
	symlink string
	// socket is the listening socket bound to a socket node, or nil.
	socket *unixSocket
	// tty is the terminal of /dev/console or a /dev/pts node, or nil.
	tty *tty
	// hostPath is the root filesystem path of a host-backed node, or empty.
	hostPath string
	// gen generates a synthesized file's content, which each open takes a
	// snapshot of.
	gen func() []byte
	// list updates a synthesized directory's entries before each use.
	list  func(dir *VNode)
	atime time.Time
	mtime time.Time
	ctime time.Time
	// seals holds the file seals of an in-memory regular file.
	seals uint32
}

// setSymlink points a symlink at target.
func (node *VNode) setSymlink(target string) {
	node.symlink = target
	node.size = int64(len(target))
}
