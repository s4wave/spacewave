//go:build linux
// +build linux

package fuse

import (
	"context"
	ofs "io/fs"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/pkg/errors"
)

// Inode wraps unixfs.FSHandle to provide FUSE inode calls.
//
// All requests types embed a Header, meaning that the method can inspect
// req.Pid, req.Uid, and req.Gid as necessary to implement permission checking.
//
// Manages deduplicating child inodes. Inodes are cleared when their reference
// is released.
type Inode struct {
	h        *unixfs.FSHandle
	rfs      *RootFS
	parent   *Inode
	attrFn   atomic.Pointer[func(ctx context.Context, attr *fuse.Attr) error]
	mtx      sync.Mutex
	children map[string]*Inode
}

// NewInode constructs a new inode with a reference.
func NewInode(rfs *RootFS, parent *Inode, ref *unixfs.FSHandle) *Inode {
	in := &Inode{
		h:        ref,
		rfs:      rfs,
		parent:   parent,
		children: make(map[string]*Inode),
	}
	// TODO: AddChangeCb
	// ref.AddReleaseCallback(in.handleInodeReleased)
	return in
}

// GetAttr returns attributes for a file.
/*
func (i *Inode) Getattr(
	ctx context.Context,
	fh fs.FileHandle,
	out *fuse.AttrOut,
) syscall.Errno {
	out.Mode = 0755
	return 0
}
*/

// GetNodeType returns the inode node type.
func (i *Inode) GetNodeType(ctx context.Context) (unixfs.FSCursorNodeType, error) {
	return i.h.GetNodeType(ctx)
}

// Attr fills attr with the standard metadata for the node.
//
// The FUSE library will set the inode number in attr.
//
// The result may be cached by the kernel for the duration set in Valid.
func (i *Inode) Attr(ctx context.Context, attr *fuse.Attr) error {
	err := FsOpsToAttr(ctx, i.h, attr)
	// if a handle is active, be sure to include the Size from pending writes in the Attr.
	if fn := i.attrFn.Load(); fn != nil {
		if err := (*fn)(ctx, attr); err != nil {
			return err
		}
	}
	if err != nil {
		i.rfs.logFilesystemError(err)
		err = UnixfsErrorToSyscall(err)
	}
	return err
}

// ReadDirAll handles the readdir call.
func (i *Inode) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	var out []fuse.Dirent
	err := i.h.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		out = append(out, fuse.Dirent{})
		return DirentToFuseDirent(ent, &out[len(out)-1])
	})
	if err != nil {
		i.rfs.logFilesystemError(err)
		err = UnixfsErrorToSyscall(err)
	}
	return out, err
}

// Lookup should find a direct child of a directory by the child's name. If the
// entry does not exist, it should return ENOENT and optionally set a
// NegativeTimeout in `out`. If it does exist, it should return attribute data
// in `out` and return the Inode for the child. A new inode can be created using
// `Inode.NewInode`. The new Inode will be added to the FS tree automatically if
// the return status is OK.
//
// The input to a Lookup is {parent directory, name string}.
//
// Lookup, if successful, must return an *Inode. Once the Inode is returned to
// the kernel, the kernel can issue further operations, such as Open or Getxattr
// on that node.
//
// FUSE supports other operations that modify the namespace. For example, the
// Symlink, Create, Mknod, Link methods all create new children in directories.
// Hence, they also return *Inode and must populate their fuse.EntryOut
// arguments.
func (i *Inode) Lookup(
	ctx context.Context,
	req *fuse.LookupRequest,
	resp *fuse.LookupResponse,
) (fs.Node, error) {
	name := req.Name
	return i.lookupNodeByName(ctx, name, &resp.Attr)
}

// lookupNodeByName looks up a child node by name.
// deduplicates child concurrently
// attr can be nil
func (i *Inode) lookupNodeByName(
	ctx context.Context,
	name string,
	attr *fuse.Attr,
) (*Inode, error) {
	// check if the child already exists
	i.mtx.Lock()
	ci, ciOk := i.children[name]
	if ciOk {
		if ci.h.CheckReleased() {
			delete(i.children, name)
			ciOk = false
			ci = nil
		}
	}
	i.mtx.Unlock()
	if ciOk {
		if attr != nil {
			if err := FsOpsToAttr(ctx, ci.h, attr); err != nil {
				i.rfs.logFilesystemError(err)
				return nil, UnixfsErrorToSyscall(err)
			}
		}
		return ci, nil
	}

	childRef, err := i.h.Lookup(ctx, name)
	if err != nil {
		if err == unixfs_errors.ErrNotExist {
			return nil, syscall.ENOENT
		}

		i.rfs.logFilesystemError(err)
		return nil, UnixfsErrorToSyscall(err)
	}

	// determine information for Entryout
	if attr != nil {
		if err := FsOpsToAttr(ctx, childRef, attr); err != nil {
			childRef.Release()
			if err == unixfs_errors.ErrNotExist {
				return nil, syscall.ENOENT
			}
			i.rfs.logFilesystemError(err)
			return nil, UnixfsErrorToSyscall(err)
		}
	}

	i.mtx.Lock()
	ci, ciOk = i.children[name]
	if ciOk {
		if ci.h.CheckReleased() {
			// re-create and swap
			ciOk = false
		}
	}
	if !ciOk {
		ci = NewInode(i.rfs, i, childRef)
		i.children[name] = ci
	} else {
		// duplicate reference
		childRef.Release()
	}
	i.mtx.Unlock()
	return ci, nil
}

// Mkdir creates a directory returning the new inode reference.
func (i *Inode) Mkdir(
	ctx context.Context,
	req *fuse.MkdirRequest,
) (fs.Node, error) {
	name := req.Name
	ts := time.Now()
	err := i.h.Mknod(
		ctx,
		true,
		[]string{name},
		unixfs.NewFSCursorNodeType_Dir(),
		0,
		ts,
	)
	if err != nil {
		i.rfs.logFilesystemError(err)
		return nil, UnixfsErrorToSyscall(err)
	}

	// directory created, now return result of Lookup
	return i.lookupNodeByName(ctx, name, nil)
}

// Mknod creates a node in a directory.
func (i *Inode) Mknod(
	ctx context.Context,
	req *fuse.MknodRequest,
) (fs.Node, error) {
	name, mode := req.Name, req.Mode
	nodType, err := unixfs.FileModeToNodeType(mode)
	if err != nil {
		i.rfs.logFilesystemError(err)
		return nil, UnixfsErrorToSyscall(err)
	}

	ts := time.Now()
	err = i.h.Mknod(ctx, true, []string{name}, nodType, 0, ts)
	if err != nil {
		i.rfs.logFilesystemError(err)
		return nil, UnixfsErrorToSyscall(err)
	}

	return i.lookupNodeByName(ctx, name, nil)
}

// Symlink creates a new symbolic link in the receiver, which must be a directory.
func (i *Inode) Symlink(ctx context.Context, req *fuse.SymlinkRequest) (fs.Node, error) {
	ts := time.Now()
	linkName, targetPath := req.NewName, req.Target
	tgtSplit, tgtAbsolute := unixfs.SplitPath(targetPath)
	if err := i.h.Symlink(ctx, true, linkName, tgtSplit, tgtAbsolute, ts); err != nil {
		i.rfs.logFilesystemError(err)
		return nil, UnixfsErrorToSyscall(err)
	}

	return i.lookupNodeByName(ctx, linkName, nil)
}

// Readlink reads a symbolic link.
func (i *Inode) Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error) {
	linkPath, linkAbsolute, err := i.h.Readlink(ctx, "")
	if err != nil {
		return "", nil
	}
	return unixfs.JoinPath(linkPath, linkAbsolute), nil
}

// Create creates a new directory entry in the receiver, which must be a
// directory.
func (i *Inode) Create(
	ctx context.Context,
	req *fuse.CreateRequest,
	resp *fuse.CreateResponse,
) (fs.Node, fs.Handle, error) {
	name := req.Name
	mode := req.Mode

	nodType, err := unixfs.FileModeToNodeType(mode)
	if err != nil {
		i.rfs.logFilesystemError(err)
		return nil, nil, UnixfsErrorToSyscall(err)
	}

	flags := req.Flags
	checkIfExists := flags&fuse.OpenExclusive != 0

	ts := time.Now()
	err = i.h.Mknod(ctx, checkIfExists, []string{name}, nodType, mode&ofs.ModePerm, ts)
	if err != nil {
		i.rfs.logFilesystemError(err)
		return nil, nil, UnixfsErrorToSyscall(err)
	}

	childNode, err := i.lookupNodeByName(ctx, name, &resp.Attr)
	if err != nil {
		// already in syscall format
		return nil, nil, err
	}

	return childNode, NewHandle(childNode, req.Flags), nil
}

// Rename moves an inode from one location to another.
func (i *Inode) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	// newDir is conveniently provided by FUSE
	toDir, ok := newDir.(*Inode)
	if !ok {
		i.rfs.logFilesystemError(errors.New("rename called with unrecognized fs.Node type"))
		return syscall.EINVAL
	}
	_ = toDir

	// open a handle for the inode we want to move
	mvNode, err := i.h.Lookup(ctx, req.OldName)
	if err != nil {
		i.rfs.logFilesystemError(err)
		return UnixfsErrorToSyscall(err)
	}
	// release the handle to the node when done
	defer mvNode.Release()

	ts := time.Now()
	tgtNode := toDir.h
	if err := mvNode.Rename(ctx, tgtNode, req.NewName, ts); err != nil {
		i.rfs.logFilesystemError(err)
		return UnixfsErrorToSyscall(err)
	}

	// release old target location
	oldDest, oldDestOk := toDir.children[req.NewName]
	if oldDestOk {
		oldDest.releaseRecursive()
	}

	// move the handle + children inodes to the destination.
	oldChild, oldChildOk := i.children[req.OldName]
	if oldChildOk {
		delete(i.children, req.OldName)
		toDir.children[req.NewName] = oldChild
	} else if oldDestOk {
		delete(toDir.children, req.NewName)
	}

	// flush data for destination dir
	/*
		 toDir.rfs.server.InvalidateNodeData(toDir)

		// clear the parent inode cache
		if i.parent != nil {
			_ = i.rfs.server.InvalidateNodeData(i.parent)
		}
	*/

	return nil
}

// Setattr sets the standard metadata for the receiver.
//
// Note, this is also used to communicate changes in the size of
// the file, outside of Writes.
//
// req.Valid is a bitmask of what fields are actually being set.
// For example, the method should not change the mode of the file
// unless req.Valid.Mode() is true.
func (i *Inode) Setattr(
	ctx context.Context,
	req *fuse.SetattrRequest,
	resp *fuse.SetattrResponse,
) error {
	info, err := i.h.GetFileInfo(ctx)
	if err != nil {
		return err
	}

	setMtime := req.Valid.Mtime()
	useMtime := time.Now()
	if setMtime {
		useMtime = req.Mtime
	}

	if req.Valid.Size() {
		oldSize := info.Size()
		setSize := req.Size
		if uint64(oldSize) != setSize {
			err = i.h.Truncate(ctx, setSize, useMtime)
			if err != nil {
				i.rfs.logFilesystemError(err)
				return UnixfsErrorToSyscall(err)
			}
		}
	}

	if req.Valid.Mode() {
		oldType := info.Mode() & ofs.ModeType
		setType := req.Mode & ofs.ModeType
		if oldType != setType {
			return errors.New("TODO setattr: change node type")
		}

		oldPerms := info.Mode() & ofs.ModePerm
		setPerms := req.Mode & ofs.ModePerm
		if oldPerms != setPerms {
			err = i.h.SetPermissions(ctx, setPerms, useMtime)
			if err != nil {
				i.rfs.logFilesystemError(err)
				return UnixfsErrorToSyscall(err)
			}
		} else {
			// update mtime anyway
			setMtime = true
		}
	}

	if setMtime {
		err := i.h.SetModTimestamp(ctx, useMtime)
		if err != nil {
			return err
		}
	}

	return nil
}

// Open opens the receiver. After a successful open, a client
// process has a file descriptor referring to this Handle.
//
// Open can also be also called on non-files. For example,
// directories are Opened for ReadDir or fchdir(2).
//
// If this method is not implemented, the open will always
// succeed, and the Node itself will be used as the Handle.
//
// XXX note about access.  XXX OpenFlags.
func (i *Inode) Open(
	ctx context.Context,
	req *fuse.OpenRequest,
	resp *fuse.OpenResponse,
) (fs.Handle, error) {
	return NewHandle(i, req.Flags), nil
}

// Remove removes the entry with the given name from the receiver, which must be
// a directory. The entry to be removed may correspond to a file (unlink) or to
// a directory (rmdir).
func (i *Inode) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	ts := time.Now()
	err := i.h.Remove(ctx, []string{req.Name}, ts)
	if err != nil {
		i.rfs.logFilesystemError(err)
		err = UnixfsErrorToSyscall(err)
	}
	return err
}

// Fsync finishes and synchronizes any ongoing i/o ops.
func (i *Inode) Fsync(ctx context.Context, req *fuse.FsyncRequest) error {
	// NOTE: Flush should also be called on the Handle.
	// All other operations are SYNC by default.
	return nil
}

// Forget about this node. This node will not receive further
// method calls.
//
// Forget is not necessarily seen on unmount, as all nodes are
// implicitly forgotten as part of the unmount.
func (i *Inode) Forget() {
	// Do this in a separate goroutine to avoid locking fuse.
	go i.releaseRecursive()
}

// releaseRecursive releases the FSHandles recursively.
func (i *Inode) releaseRecursive() {
	stk := []*Inode{i}
	var toRelease []*unixfs.FSHandle
	for len(stk) != 0 {
		v := stk[len(stk)-1]
		stk = stk[:len(stk)-1]

		toRelease = append(toRelease, i.h)
		for _, child := range v.children {
			stk = append(stk, child)
		}
	}
	for i := len(toRelease) - 1; i >= 0; i-- {
		toRelease[i].Release()
	}
}

// handleInodeChanged handles when the unixfs inode changed.
func (i *Inode) handleInodeChanged(ch *unixfs.FSCursorChange) bool {
	// create new goroutine to avoid locking anything
	go func() {
		if ch.Released || (ch.Offset == 0 && ch.Size == 0) {
			// completely flush this inode
			_ = i.rfs.server.InvalidateNodeData(i)
			return
		}
		// invalidate parent entry
		if i.parent != nil {
			name := i.h.GetName()
			if name != "" {
				_ = i.rfs.server.InvalidateEntry(i.parent, name)
			}
		}
	}()
	return true
}

// _ is a type assertion
var (
	// _ fs.NodeGetattrer is unnecessary: the FUSE library will fill values from Attr().

	// Methods returning Node should take care to return the same Node when the
	// result is logically the same instance. Without this, each Node will get a
	// new NodeID, causing spurious cache invalidations, extra lookups and
	// aliasing anomalies. This may not matter for a simple, read-only
	// filesystem.
	_ fs.Node = ((*Inode)(nil))

	_ fs.NodeRemover         = ((*Inode)(nil))
	_ fs.NodeMkdirer         = ((*Inode)(nil))
	_ fs.NodeMknoder         = ((*Inode)(nil))
	_ fs.NodeCreater         = ((*Inode)(nil))
	_ fs.NodeRenamer         = ((*Inode)(nil))
	_ fs.NodeOpener          = ((*Inode)(nil))
	_ fs.NodeRequestLookuper = ((*Inode)(nil))
	_ fs.NodeSetattrer       = ((*Inode)(nil))
	_ fs.NodeForgetter       = ((*Inode)(nil))
	_ fs.NodeSymlinker       = ((*Inode)(nil))
	_ fs.NodeReadlinker      = ((*Inode)(nil))
	_ fs.NodeFsyncer         = ((*Inode)(nil))

	_ fs.HandleReadDirAller = ((*Inode)(nil))

	_ unixfs.FSCursorChangeCb = ((*Inode)(nil)).handleInodeChanged
)
