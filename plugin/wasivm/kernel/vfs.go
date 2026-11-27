package kernel

import (
	"bytes"
	"cmp"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/util"
)

// VFS is the kernel's filesystem: an in-memory tree over an optional root
// filesystem. A lookup that misses the tree loads the node from the same path
// in the root filesystem, so in-memory nodes overlay it.
type VFS struct {
	root *VNode
	// nextIno is the inode number of the next node.
	nextIno uint64
	// host is the root filesystem, or nil for a tree held only in memory.
	host billy.Filesystem
}

// newVFS creates a VFS with an empty root directory.
func newVFS() *VFS {
	// Create the root directory as the first node.
	v := &VFS{nextIno: 1}
	v.root = v.newDir(0o755)
	return v
}

// allocIno returns a fresh inode number.
func (v *VFS) allocIno() uint64 {
	ino := v.nextIno
	v.nextIno++
	return ino
}

// newNode creates a detached node of mode with a fresh inode number.
func (v *VFS) newNode(mode uint32) *VNode {
	now := time.Now()
	return &VNode{ino: v.allocIno(), mode: mode, nlink: 1, atime: now, mtime: now, ctime: now, seals: sealSeal}
}

// newDir creates a detached empty directory.
func (v *VFS) newDir(perm uint32) *VNode {
	node := v.newNode(sIFDIR | perm)
	node.nlink = 2
	node.children = make(map[string]*VNode)
	return node
}

// link enters node in directory parent as name.
func link(parent *VNode, name string, node *VNode) {
	node.parent = parent
	parent.children[name] = node
}

// mkNode creates a node of mode under parent. rdev is the device number of a
// device node.
func (v *VFS) mkNode(parent *VNode, name string, mode uint32, rdev uint64) *VNode {
	node := v.newNode(mode)
	node.rdev = rdev
	link(parent, name, node)
	return node
}

// mkDir creates a directory under parent.
func (v *VFS) mkDir(parent *VNode, name string) *VNode {
	// Link an empty directory, whose ".." entry adds a link to parent.
	node := v.newDir(0o755)
	link(parent, name, node)
	parent.nlink++
	return node
}

// mkGenFile creates a read-only file under parent whose content gen
// generates at each open.
func (v *VFS) mkGenFile(parent *VNode, name string, gen func() []byte) *VNode {
	node := v.mkNode(parent, name, sIFREG|0o444, 0)
	node.gen = gen
	return node
}

// mkFile creates a regular file under parent holding a copy of content.
func (v *VFS) mkFile(parent *VNode, name string, content []byte, perm uint32) *VNode {
	node := v.mkNode(parent, name, sIFREG|perm&0o7777, 0)
	node.data = bytes.Clone(content)
	node.size = int64(len(content))
	return node
}

// Device numbers of the character devices in /dev, the major number in the
// high byte and the minor in the low.
const (
	devNull    = 1<<8 | 3
	devZero    = 1<<8 | 5
	devURandom = 1<<8 | 9
	devTTY     = 5 << 8
	devConsole = 5<<8 | 1
)

// mkCharDev creates a character device under parent with device number rdev.
func (v *VFS) mkCharDev(parent *VNode, name string, rdev uint64) *VNode {
	return v.mkNode(parent, name, sIFCHR|0o666, rdev)
}

// mkSymlink creates a symbolic link under parent pointing to target.
func (v *VFS) mkSymlink(parent *VNode, name, target string) *VNode {
	node := v.mkNode(parent, name, sIFLNK|0o777, 0)
	node.setSymlink(target)
	return node
}

// vnodeFromHost creates a node for a root filesystem path without following
// a final symlink. It returns nil when the path does not exist.
func (v *VFS) vnodeFromHost(hostPath string) *VNode {
	info, err := v.host.Lstat(hostPath)
	if err != nil {
		return nil
	}
	return v.vnodeFromInfo(hostPath, info)
}

// vnodeFromInfo creates a node for the root filesystem path hostPath from its
// Lstat info.
func (v *VFS) vnodeFromInfo(hostPath string, info os.FileInfo) *VNode {
	// Create the node with the path's type, size and times.
	mode := uint32(info.Mode().Perm())
	switch {
	case info.IsDir():
		mode |= sIFDIR
	case info.Mode()&os.ModeSymlink != 0:
		mode |= sIFLNK
	default:
		mode |= sIFREG
	}
	node := v.newNode(mode)
	node.size = info.Size()
	node.hostPath = hostPath
	node.atime, node.mtime, node.ctime = info.ModTime(), info.ModTime(), info.ModTime()

	// Add what its type needs.
	switch mode & sIFMT {
	case sIFDIR:
		node.children = make(map[string]*VNode)
		node.nlink = 2
	case sIFLNK:
		if target, err := v.host.Readlink(hostPath); err == nil {
			node.setSymlink(target)
		}
	}
	return node
}

// refreshHost updates a host-backed node's size and modification time from
// the root filesystem.
func (v *VFS) refreshHost(node *VNode) {
	if node.hostPath == "" {
		return
	}
	if info, err := v.host.Lstat(node.hostPath); err == nil {
		node.size = info.Size()
		node.mtime = info.ModTime()
	}
}

// refreshDirChildren updates a synthesized directory's entries, or loads the
// missing entries of a host-backed directory while preserving in-memory
// overlays.
func (v *VFS) refreshDirChildren(node *VNode) {
	switch {
	case node == nil || node.mode&sIFMT != sIFDIR:
	case node.list != nil:
		node.list(node)
	case node.hostPath != "":
		v.loadHostChildren(node, node.hostPath)
	}
}

// loadHostChildren adds the entries of the root filesystem directory at
// hostPath to node, keeping children node already has.
func (v *VFS) loadHostChildren(node *VNode, hostPath string) {
	// List the directory.
	entries, err := v.host.ReadDir(hostPath)
	if err != nil {
		return
	}
	if node.children == nil {
		node.children = make(map[string]*VNode)
	}

	// Add the entries the tree lacks, from the info the listing returned.
	for _, entry := range entries {
		name := entry.Name()
		if node.children[name] != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		child := v.vnodeFromInfo(hostChildPath(hostPath, name), info)
		child.parent = node
		node.children[name] = child
	}
}

// readFile returns a copy of a whole file, read from the root filesystem
// when the file is host-backed.
func (v *VFS) readFile(node *VNode) ([]byte, error) {
	if node.hostPath != "" {
		return util.ReadFile(v.host, node.hostPath)
	}
	return bytes.Clone(node.data), nil
}

// truncateHost resizes a root filesystem file.
func (v *VFS) truncateHost(hostPath string, size int64) error {
	// Open the file, resize it and report the first error.
	f, err := v.host.OpenFile(hostPath, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	err = f.Truncate(size)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// chtimesHost sets a root filesystem file's times. A root filesystem
// without times support keeps them only in the VNode.
func (v *VFS) chtimesHost(hostPath string, atime, mtime time.Time) error {
	ch, ok := v.host.(billy.Change)
	if !ok {
		return nil
	}
	return ch.Chtimes(hostPath, atime, mtime)
}

// chmodHost sets the permission bits of a root filesystem file, which
// vnodeFromHost reads back. A root filesystem that cannot change modes keeps
// them.
func (v *VFS) chmodHost(hostPath string, mode uint32) error {
	ch, ok := v.host.(billy.Change)
	if !ok {
		return nil
	}
	return ch.Chmod(hostPath, os.FileMode(mode&0o777))
}

// hostChildPath joins a root filesystem directory path and an entry name.
func hostChildPath(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}

// lookup returns the node at an absolute path, following symlinks, or nil.
func (v *VFS) lookup(path string) *VNode {
	return v.lookupDepth(path, 0)
}

// maxSymlinkDepth is how many symlinks a lookup follows before it fails, as
// Linux fails with ELOOP.
const maxSymlinkDepth = 40

// lookupDepth returns the node at path after following depth symlinks, or
// nil.
func (v *VFS) lookupDepth(path string, depth int) *VNode {
	// Walk the components from the root.
	if depth > maxSymlinkDepth {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	cur := v.root
	for i, name := range parts {
		switch name {
		case "", ".":
			continue
		case "..":
			cur = cmp.Or(cur.parent, cur)
			continue
		}
		child := v.child(cur, name)
		if child == nil {
			return nil
		}

		// Continue from a symlink's target with the rest of the path.
		if child.mode&sIFMT == sIFLNK {
			target := child.symlink
			if !strings.HasPrefix(target, "/") {
				target = v.nodePath(cur) + "/" + target
			}
			rest := append([]string{target}, parts[i+1:]...)
			return v.lookupDepth(cleanPath(strings.Join(rest, "/")), depth+1)
		}
		cur = child
	}
	return cur
}

// lookupNoFollow returns the node at an absolute path, or nil. A symlink at
// the end of the path is the node itself.
func (v *VFS) lookupNoFollow(path string) *VNode {
	// Follow the parent's path, then take the last entry as it is.
	if path == "" || path == "/" {
		return v.root
	}
	dir, name := splitPath(path)
	parent := v.lookup(dir)
	if parent == nil {
		return nil
	}
	return v.child(parent, name)
}

// child returns the entry name of directory dir, loading it from the root
// filesystem when the tree lacks it, or nil.
func (v *VFS) child(dir *VNode, name string) *VNode {
	// Use the tree's entry, updating a synthesized directory first.
	if dir.list != nil {
		dir.list(dir)
	}
	if child := dir.children[name]; child != nil || v.host == nil || dir.list != nil {
		return child
	}

	// Load the entry from the root filesystem.
	child := v.vnodeFromHost(hostChildPath(v.nodePath(dir), name))
	if child == nil {
		return nil
	}
	if dir.children == nil {
		dir.children = make(map[string]*VNode)
	}
	child.parent = dir
	dir.children[name] = child
	return child
}

// nodePath returns the absolute path of node by walking its parents.
func (v *VFS) nodePath(node *VNode) string {
	// Find each node's name in its parent, from node up to the root.
	var names []string
	for cur := node; cur != v.root && cur.parent != nil; cur = cur.parent {
		names = append(names, nameIn(cur.parent, cur))
	}

	// Join them from the root down.
	slices.Reverse(names)
	return "/" + strings.Join(names, "/")
}

// nameIn returns the name of node in directory dir, or "" when dir does not
// hold it.
func nameIn(dir, node *VNode) string {
	for name, child := range dir.children {
		if child == node {
			return name
		}
	}
	return ""
}

// mkDirAll creates the in-memory directories along an absolute path and
// returns the last directory.
func (v *VFS) mkDirAll(path string) *VNode {
	cur := v.root
	for name := range strings.SplitSeq(path, "/") {
		switch {
		case name == "" || name == ".":
		case cur.children[name] != nil:
			cur = cur.children[name]
		default:
			cur = v.mkDir(cur, name)
		}
	}
	return cur
}

// addFile creates an in-memory file at an absolute path, creating its parent
// directories.
func (v *VFS) addFile(path string, content []byte, perm uint32) *VNode {
	dir, name := splitPath(path)
	return v.mkFile(v.mkDirAll(dir), name, content, perm)
}

// splitPath splits an absolute path into its directory and base name.
func splitPath(p string) (string, string) {
	dir, name := path.Split(strings.TrimSuffix(p, "/"))
	if dir != "/" {
		dir = strings.TrimSuffix(dir, "/")
	}
	return cmp.Or(dir, "/"), name
}

// cleanPath returns the shortest absolute path equivalent to path, resolving
// "." and ".." lexically.
func cleanPath(p string) string {
	return path.Clean("/" + p)
}

// resolvePath resolves path against the directory of descriptor dirfd, or
// against cwd for AT_FDCWD or an unknown descriptor.
func resolvePath(cwd string, fds map[uint32]*OpenFile, dirfd uint32, p string) string {
	if strings.HasPrefix(p, "/") {
		return cleanPath(p)
	}
	base := cwd
	if of, ok := fds[dirfd]; ok && dirfd != atFDCWD {
		base = of.path
	}
	return cleanPath(base + "/" + p)
}
