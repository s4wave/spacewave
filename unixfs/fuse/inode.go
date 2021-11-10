package fuse

import (
	"context"
	"errors"
	ofs "io/fs"
	"sync"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
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
// Fields with reasonable defaults are prepopulated. For example,
// all times are set to a fixed moment when the program started.
//
// If Inode is left as 0, a dynamic inode number is chosen.
//
// The result may be cached for the duration set in Valid.
func (i *Inode) Attr(ctx context.Context, attr *fuse.Attr) error {
	err := FsOpsToAttr(ctx, i.h, attr)
	if err != nil {
		i.rfs.logFilesystemError(err)
		err = UnixfsErrorToSyscall(err)
	}
	return err
}

// ReadDirAll handles the readdir call.
func (i *Inode) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	var out []fuse.Dirent
	err := i.h.ReaddirAll(ctx, func(ent unixfs.FSCursorDirent) error {
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
			i.rfs.logFilesystemError(err)
			childRef.Release()
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
	name := req.Name
	mode := req.Mode
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
	err = i.h.Mknod(ctx, checkIfExists, []string{name}, nodType, 0, ts)
	if err != nil {
		i.rfs.logFilesystemError(err)
		return nil, nil, UnixfsErrorToSyscall(err)
	}

	childNode, err := i.lookupNodeByName(ctx, name, &resp.Attr)
	if err != nil {
		i.rfs.logFilesystemError(err)
		return nil, nil, UnixfsErrorToSyscall(err)
	}

	return childNode, NewHandle(childNode, req.Flags), nil
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
				return err
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
				return err
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

// Forget about this node. This node will not receive further
// method calls.
//
// Forget is not necessarily seen on unmount, as all nodes are
// implicitly forgotten as part of the unmount.
func (i *Inode) Forget() {
	// Do this in a separate goroutine to avoid locking fuse.
	go i.h.Release()
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
	// _ fs.NodeGetattrer is unnecessary!

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
	_ fs.NodeOpener          = ((*Inode)(nil))
	_ fs.NodeRequestLookuper = ((*Inode)(nil))
	_ fs.NodeSetattrer       = ((*Inode)(nil))
	_ fs.NodeForgetter       = ((*Inode)(nil))

	// _ fs.NodeRenamer = ((*Inode)(nil))
	// _ fs.NodeFsyncer         = ((*Inode)(nil))

	_ fs.HandleReadDirAller = ((*Inode)(nil))

	_ unixfs.FSCursorChangeCb = ((*Inode)(nil)).handleInodeChanged
)
