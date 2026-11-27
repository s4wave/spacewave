package kernel

import (
	"bytes"
	"maps"
	"slices"
	"time"
)

// The extended attribute syscalls store Linux xattr name and value pairs on
// the node. The path, link and descriptor variants share the node operations,
// so the create and replace flags, size queries, missing attributes and the
// list encoding behave the same through each variant.

// sysSetxattr sets an extended attribute on a path, following symlinks.
func (p *Process) sysSetxattr(pathPtr, namePtr, valuePtr, size, flags uint32) uint64 {
	node, errno := p.xattrPathNode(pathPtr, false)
	if errno != 0 {
		return errno
	}
	return p.setNodeXattr(node, namePtr, valuePtr, size, flags)
}

// sysGetxattr gets an extended attribute from a path, following symlinks.
func (p *Process) sysGetxattr(pathPtr, namePtr, valuePtr, size uint32) uint64 {
	node, errno := p.xattrPathNode(pathPtr, false)
	if errno != 0 {
		return errno
	}
	return p.getNodeXattr(node, namePtr, valuePtr, size)
}

// sysListxattr lists the extended attribute names of a path, following
// symlinks.
func (p *Process) sysListxattr(pathPtr, listPtr, size uint32) uint64 {
	node, errno := p.xattrPathNode(pathPtr, false)
	if errno != 0 {
		return errno
	}
	return p.listNodeXattrs(node, listPtr, size)
}

// xattrPathNode returns the node at the path at pathPtr. With noFollow, a
// symlink at the end of the path is the node itself.
func (p *Process) xattrPathNode(pathPtr uint32, noFollow bool) (*VNode, uint64) {
	// Resolve the path.
	path := readCString(p.inst.Memory(), pathPtr)
	if path == "" {
		return nil, negErrno(ENOENT)
	}
	resolved := resolvePath(p.cwd, p.fds, atFDCWD, path)

	// Look up the node.
	lookup := p.kernel.vfs.lookup
	if noFollow {
		lookup = p.kernel.vfs.lookupNoFollow
	}
	node := lookup(resolved)
	if node == nil {
		return nil, negErrno(ENOENT)
	}
	return node, 0
}

// readXattrName reads the attribute name at namePtr. An empty name is
// invalid.
func (p *Process) readXattrName(namePtr uint32) (string, uint64) {
	name := readCString(p.inst.Memory(), namePtr)
	if name == "" {
		return "", negErrno(EINVAL)
	}
	return name, 0
}

// setNodeXattr sets the attribute at namePtr on node to the size bytes at
// valuePtr. XATTR_CREATE fails when the attribute exists, and XATTR_REPLACE
// fails when it does not.
func (p *Process) setNodeXattr(node *VNode, namePtr, valuePtr, size, flags uint32) uint64 {
	// Read the name and value.
	if flags&^(xattrCreate|xattrReplace) != 0 || flags == xattrCreate|xattrReplace {
		return negErrno(EINVAL)
	}
	name, errno := p.readXattrName(namePtr)
	if errno != 0 {
		return errno
	}
	dat, ok := p.inst.Memory().Read(valuePtr, size)
	if !ok || (size != 0 && valuePtr == 0) {
		return negErrno(EFAULT)
	}

	// Check the flags against the current attribute.
	_, exists := node.xattrs[name]
	if flags&xattrCreate != 0 && exists {
		return negErrno(EEXIST)
	}
	if flags&xattrReplace != 0 && !exists {
		return negErrno(ENODATA)
	}

	// Store the value.
	if node.xattrs == nil {
		node.xattrs = make(map[string][]byte)
	}
	node.xattrs[name] = bytes.Clone(dat)
	node.ctime = time.Now()
	return 0
}

// getNodeXattr writes the value of the attribute at namePtr to valuePtr and
// returns its length. A size of zero returns the length alone.
func (p *Process) getNodeXattr(node *VNode, namePtr, valuePtr, size uint32) uint64 {
	// Find the attribute.
	name, errno := p.readXattrName(namePtr)
	if errno != 0 {
		return errno
	}
	dat, ok := node.xattrs[name]
	if !ok {
		return negErrno(ENODATA)
	}

	// Write its value.
	return p.writeSized(valuePtr, size, dat)
}

// listNodeXattrs writes node's attribute names to listPtr, each followed by
// a zero byte, and returns the list's length. A size of zero returns the
// length alone.
func (p *Process) listNodeXattrs(node *VNode, listPtr, size uint32) uint64 {
	return p.writeSized(listPtr, size, node.xattrList())
}

// writeSized writes dat to a guest buffer of size bytes at ptr and returns
// its length. A size of zero returns the length alone, and a buffer too
// small for dat fails with ERANGE.
func (p *Process) writeSized(ptr, size uint32, dat []byte) uint64 {
	switch {
	case size == 0:
		return uint64(len(dat))
	case uint32(len(dat)) > size:
		return negErrno(ERANGE)
	case len(dat) != 0 && (ptr == 0 || !p.inst.Memory().Write(ptr, dat)):
		return negErrno(EFAULT)
	}
	return uint64(len(dat))
}

// xattrList returns node's attribute names in sorted order, each followed by
// a zero byte.
func (node *VNode) xattrList() []byte {
	var list []byte
	for _, name := range slices.Sorted(maps.Keys(node.xattrs)) {
		list = append(list, name...)
		list = append(list, 0)
	}
	return list
}
