package unixfs

import (
	"context"
	"sort"
	"sync/atomic"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// fsInodeTries is the maximum number of tries for an operation.
// unlikely to reach this many tries
const fsInodeTries = 100

// fsInode is internal tracking of a location in a FS.
//
// the inode will be released if:
//  - there is any error fetching/refreshing the parent cursors
//  - the underlying FSCursor is released.
//  - there are 0 references to the node and 0 child nodes
type fsInode struct {
	// isReleased is a uint32 atomic int
	isReleased uint32
	// relErr is the error set when releasing.
	// do not access until isReleased is true
	relErr error
	// f is the attached FS
	// immutable
	f *FS
	// parent is the inode which created this inode
	// immutable
	parent *fsInode
	// name is the name associated with the inode
	// immutable
	name string

	// below fields are guarded by f.waitSema

	// refs is the list of inode refs
	refs []*FSHandle
	// children contains any child inodes
	// sorted by name
	children []*fsInode
	// fsCursors contains the current fs cursor instances.
	// multiple can be set if one cursor proxies to another.
	// nil until resolved
	fsCursors []FSCursor
	// fsOps contains the current fs ops instance.
	// nil until resolved (after fsCursors)
	fsOps FSCursorOps
	// fsOpsWait is closed when the fs ops lookup is complete
	// nil if there is no lookup in progress
	fsOpsWait chan struct{}
}

// newFsInode constructs a new FS inode.
// if parent is nil, indicates root node
func newFsInode(f *FS, parent *fsInode, name string) *fsInode {
	return &fsInode{
		f:      f,
		parent: parent,
		name:   name,
	}
}

// accessInodeCb is a callback for accessInode
type accessInodeCb func(ops FSCursorOps) error

// checkReleased checks if released without locking anything.
// if released (true), also returns any error set when releasing.
func (i *fsInode) checkReleased() bool {
	return atomic.LoadUint32(&i.isReleased) == 1
}

// checkReleasedWithErr checks if the node was released
// if released (true), also returns any error set when releasing.
// returns ErrReleased if no rel err was set.
func (i *fsInode) checkReleasedWithErr() (bool, error) {
	if !i.checkReleased() {
		return false, nil
	}
	if err := i.relErr; err != nil {
		return true, err
	}
	return true, unixfs_errors.ErrReleased
}

// addReference adds a new FSHandle pointing to this location
// caller must have checked if the inode is released and locked waitSema
func (i *fsInode) addReference() (*FSHandle, error) {
	_, relErr := i.checkReleasedWithErr()
	if relErr != nil {
		return nil, relErr
	}

	ref := newFSHandle(i)
	i.refs = append(i.refs, ref)
	return ref, nil
}

// lookup attempts to lookup a directory entry returning a new FSHandle.
func (i *fsInode) lookup(ctx context.Context, name string) (*FSHandle, error) {
	if err := i.f.waitSema.Acquire(ctx, 1); err != nil {
		return nil, err
	}

	// fast path: inode child already exists
	childInode, _ := i.findChildInode(name, true)
	if childInode != nil {
		nref, err := childInode.addReference()
		i.f.waitSema.Release(1)
		return nref, err
	}

	// fast-ish path: ops is already resolved, access it
	ops := i.fsOps
	if ops != nil && ops.CheckReleased() {
		ops = nil
		i.fsOps = nil
	}
	i.f.waitSema.Release(1)

	var lcursor FSCursor
	accessLookup := func(ops FSCursorOps) error {
		var err error
		lcursor, err = ops.Lookup(ctx, name)
		return err
	}

	if ops != nil {
		// ignore error if this doesn't work first try.
		_ = accessLookup(ops)
	}

	if lcursor == nil || lcursor.CheckReleased() {
		// slow path: resolve parent + this node again
		if err := i.accessInode(ctx, accessLookup); err != nil {
			// if cursor != nil ensure we release it
			if lcursor != nil {
				lcursor.Release()
			}
			return nil, err
		}
	}
	if lcursor == nil {
		return nil, unixfs_errors.ErrNotExist
	}

	// we have the child cursor, now create the child inode.
	if err := i.f.waitSema.Acquire(ctx, 1); err != nil {
		return nil, err
	}

	// nChild is the new child inode
	nChild := newFsInode(i.f, i, name)
	nChild.fsCursors = []FSCursor{lcursor}

	// get insert idx
	childInode, insertIdx := i.findChildInode(name, false)
	if childInode != nil {
		if childInode.checkReleased() {
			// insert new inode at index
			i.children[insertIdx] = nChild
		} else {
			// race: inode was resolved while we were working.
			// throw out our copy and use theirs.
			nChild.releaseWithChildrenLocked(nil)
			nref, err := childInode.addReference()
			i.f.waitSema.Release(1)
			return nref, err
		}
	}

	// child inode not found, insert at insertidx.
	if insertIdx < len(i.children) {
		i.children = append(i.children, nil)
		copy(i.children[insertIdx+1:], i.children[insertIdx:])
		i.children[insertIdx] = nChild
	} else {
		i.children = append(i.children, nChild)
	}

	nref, err := nChild.addReference()
	i.f.waitSema.Release(1)
	return nref, err
}

// findChildInode looks for an existing non-released child by name.
// caller must hold waitSema
// returns nil, insertIdx, error
func (i *fsInode) findChildInode(name string, checkReleased bool) (*fsInode, int) {
	idx := sort.Search(len(i.children), func(ix int) bool {
		return i.children[ix].name >= name
	})
	if idx < len(i.children) && i.children[idx].name == name {
		child := i.children[idx]
		if checkReleased && child.checkReleased() {
			// remove child
			// delete (from slicetricks)
			i.children = append(i.children[:idx], i.children[idx+1:]...)
		} else {
			return child, idx
		}
	}
	return nil, idx
}

// removeRefLocked removes a ref from the refs list.
// if the refs list is now empty, calls releaseIfNecessary.
// caller must hold fs waitSema
func (i *fsInode) removeRefLocked(h *FSHandle) {
	if len(i.refs) == 0 {
		return
	}
	for ix := 0; ix < len(i.refs); ix++ {
		if i.refs[ix] == h {
			i.refs[ix] = i.refs[len(i.refs)-1]
			i.refs[len(i.refs)-1] = nil
			i.refs = i.refs[:len(i.refs)-1]
			break
		}
	}
	if len(i.refs) == 0 {
		i.refs = nil
		_ = i.releaseIfNecessary()
	}
}

// checkCursorsLocked checks all cursors to see if they are released.
// caller must hold fs waitSema
func (i *fsInode) checkCursorsLocked() {
	for len(i.fsCursors) != 0 {
		next := i.fsCursors[len(i.fsCursors)-1]
		if !next.CheckReleased() {
			// not released: stop
			break
		}

		// clear fsOps and remove this cursor
		i.fsOps = nil
		i.fsCursors[len(i.fsCursors)-1] = nil
		i.fsCursors = i.fsCursors[:len(i.fsCursors)-1]
	}
}

// release locks fs.rmtx, waitSema, and then releases the node.
// releases all children as well
// if err is set, sets the inode error to err.
func (i *fsInode) release(err error) {
	if i.checkReleased() {
		return
	}

	if err := i.f.waitSema.Acquire(context.Background(), 1); err == nil {
		defer i.f.waitSema.Release(1)
	}
	i.releaseWithChildrenLocked(err)
}

// releaseIfNecessary releases this inode if it has no refs and no children.
// caller must hold fs waitSema
// returns if the node was released
func (i *fsInode) releaseIfNecessary() bool {
	if i.checkReleased() {
		return true
	}
	if len(i.refs) != 0 || len(i.children) != 0 {
		return false
	}
	i.releaseLocked(nil)
	return true
}

// releaseLocked marks the fsInode as released.
// caller must hold fs waitSema
// caller must ensure all children are released first
// if err is set, sets the fs inode error to err
func (i *fsInode) releaseLocked(err error) {
	if i.checkReleased() {
		return
	}
	if err != nil {
		i.relErr = err
	}
	if atomic.SwapUint32(&i.isReleased, 1) != 0 {
		return
	}
	i.refs = nil
	i.children = nil
	i.fsOps = nil
	i.fsOpsWait = nil
	for ix := len(i.fsCursors) - 1; ix >= 0; ix-- {
		// use separate routine to ensure no mutex contention
		go i.fsCursors[ix].Release()
	}
	i.fsCursors = nil
}

// releaseWithChildrenLocked releases this inode and all child inodes
// caller must hold fs.rmtx and waitSema (in that order)
// if err is set, sets the fs inode error to the err
func (i *fsInode) releaseWithChildrenLocked(err error) {
	// build list of inodes to release in depth order
	var toRelease []*fsInode
	// build stack of inodes to visit
	nodStk := []*fsInode{i}
	// visit children
	for len(nodStk) != 0 {
		// pop 1 from nodStk
		next := nodStk[len(nodStk)-1]
		nodStk[len(nodStk)-1] = nil
		nodStk = nodStk[:len(nodStk)-1]

		for _, child := range next.children {
			toRelease = append(toRelease, child)
			nodStk = append(nodStk, child)
		}
	}

	// release in the correct order (bottom-up)
	for len(toRelease) != 0 {
		next := toRelease[len(toRelease)-1]
		next.releaseLocked(err)
		toRelease = toRelease[:len(toRelease)-1]
	}

	// finally release this node
	i.releaseLocked(err)
}
