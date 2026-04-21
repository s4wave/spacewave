package unixfs

import (
	"context"
	"slices"
	"sort"
	"sync/atomic"

	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/aperturerobotics/util/cqueue"
	"github.com/aperturerobotics/util/csync"
)

// fsInodeTries is the maximum number of tries for an operation.
// unlikely to reach this many tries without something being broken.
const fsInodeTries = 100

// fsInode is internal tracking of a location in a FS.
//
// the inode will be released if:
//   - there is any error fetching/refreshing the parent cursors
//   - the underlying FSCursor is released.
//   - there are 0 references to the node and 0 child nodes
type fsInode struct {
	// isReleased indicates the node is released.
	isReleased atomic.Bool
	// parent is the inode which created this inode
	// root node: parent is nil
	// immutable
	parent *fsInode
	// name is the name associated with the inode
	// immutable
	name string

	// relCbs is an atomic last-in-first-out set of callbacks
	relCbs cqueue.AtomicLIFO[func()]

	// rmtx is the read/write mutex for the inode (fields below) and children.
	// always lock parent -> child in breath-first order sorted by name.
	rmtx csync.RWMutex

	// relErr is the error set when releasing.
	relErr error
	// refs is the list of inode ref handles
	refs []*FSHandle
	// children contains any child inodes
	// sorted by name
	children []*fsInode
	// fsWait is set if a routine is currently resolving fsCursors or fsOps
	fsWait chan struct{}
	// fsCursors contains the current fs cursor instances.
	// multiple can be set if one cursor proxies to another.
	// the last element in the list is the cursor used for fsOps.
	// nil until resolved
	fsCursors []FSCursor
	// fsOps contains the current fs ops instance.
	// nil until resolved (after fsCursors)
	fsOps FSCursorOps
}

// newFsInode constructs a new FS inode.
// if parent is nil, indicates root node
func newFsInode(parent *fsInode, name string, cursors []FSCursor) *fsInode {
	return &fsInode{
		parent:    parent,
		name:      name,
		fsCursors: cursors,
	}
}

// accessInodeCb is a callback for accessInode
type accessInodeCb func(cursor FSCursor, ops FSCursorOps) error

// checkReleased checks if released without locking anything.
// if released (true), also returns any error set when releasing.
func (i *fsInode) checkReleased() bool {
	return i.isReleased.Load()
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

// addReferenceLocked adds a new FSHandle pointing to this location
// if checkReleased is false this cannot return an error.
func (i *fsInode) addReferenceLocked(checkReleased bool) (*FSHandle, error) {
	if checkReleased {
		_, relErr := i.checkReleasedWithErr()
		if relErr != nil {
			return nil, relErr
		}
	}

	ref := &FSHandle{}
	ref.inode.Store(i)
	i.refs = append(i.refs, ref)
	return ref, nil
}

// mergeReferencesLocked merges a list of Refs into the refs on the fsInode, skipping
// any released refs.
func (i *fsInode) mergeReferencesLocked(refs []*FSHandle) {
	if len(refs) == 0 {
		return
	}

	// if i is released, release all refs instead of appending.
	if i.checkReleased() {
		for _, ref := range refs {
			ref.isReleased.Store(true)
			for {
				relCb := ref.relCbs.Pop()
				if relCb == nil {
					break
				}
				relCb()
			}
		}
		return
	}

	// small ahead-of-time alloc optimization
	nlen := len(refs) + len(i.refs)
	if old := i.refs; cap(old) < nlen {
		i.refs = make([]*FSHandle, len(old), nlen)
		copy(i.refs, old)
	}

	// append any non-released refs
	for _, ref := range refs {
		if !ref.CheckReleased() {
			ref.inode.Store(i)
			i.refs = append(i.refs, ref)
		}
	}
}

// mergeWithNodeLocked merges the given inode into i, releasing the given inode
// and children of the given inode. merges the references and children
// recursively into i and children of i.
// caller must hold the mutexes
// if err is set, sets the released inode errors to the err
func (i *fsInode) mergeWithNodeLocked(node *fsInode, err error) {
	// build list of inodes to release in depth order
	toRelease := make([]*fsInode, 0, 1)
	// build stack of inodes to visit
	nodStk := []*fsInode{i}
	srcStk := []*fsInode{node}
	// visit children
	for len(nodStk) != 0 {
		// pop 1 from nodStk and srcStk
		next := nodStk[len(nodStk)-1]
		nodStk[len(nodStk)-1] = nil
		nodStk = nodStk[:len(nodStk)-1]

		src := srcStk[len(srcStk)-1]
		srcStk[len(srcStk)-1] = nil
		srcStk = srcStk[:len(srcStk)-1]

		// next = the destination location
		// src = the source location

		// copy all references from src to next
		next.mergeReferencesLocked(src.refs)
		src.refs = nil

		// copy all children from src to next
		for _, srcChild := range src.children {
			// ignore if released or if no refs + children
			if srcChild.checkReleased() || srcChild.releaseIfNecessaryLocked() {
				continue
			}

			srcChildName := srcChild.name
			childLoc, childLocIdx := next.findChildInode(srcChildName, true)
			if childLoc == nil {
				// child inode not found, insert at insertidx.
				childLoc = newFsInode(next, srcChildName, nil)
				next.children = slices.Insert(next.children, childLocIdx, childLoc)
			}

			// add the child location to the visit list
			nodStk = append(nodStk, childLoc)
			srcStk = append(srcStk, srcChild)
		}

		// release the source location
		src.children = nil
		toRelease = append(toRelease, src)
	}

	// release in the correct order (bottom-up)
	for i, v := range slices.Backward(toRelease) {
		next := v
		next.releaseLocked(err)
		toRelease = toRelease[:i]
	}
}

// clearCursorsWithChildrenLocked clears all fscursor and fscursor ops on the inode and children.
// does not release the inodes: just releases the cursors.
// caller must hold the mutexes for the node & all children
// if err is set, sets the released inode errors to the err
func (i *fsInode) clearCursorsWithChildrenLocked() {
	// build list of inodes to release in depth order
	toRelease := make([]*fsInode, 0, 1)
	// build stack of inodes to visit
	nodStk := []*fsInode{i}
	// visit children
	for len(nodStk) != 0 {
		// pop 1 from nodStk
		src := nodStk[len(nodStk)-1]
		nodStk[len(nodStk)-1] = nil
		nodStk = nodStk[:len(nodStk)-1]

		for _, srcChild := range src.children {
			// ignore if released or if no refs + children
			if srcChild.checkReleased() || srcChild.releaseIfNecessaryLocked() {
				continue
			}

			// add the child location to the visit list
			nodStk = append(nodStk, srcChild)
		}

		// add the node to the release list
		toRelease = append(toRelease, src)
	}

	// release in the correct order (bottom-up)
	for i, v := range slices.Backward(toRelease) {
		next := v
		toRelease = toRelease[:i]

		next.fsOps = nil
		for _, v := range slices.Backward(next.fsCursors) {
			v.Release()
		}
		next.fsCursors = nil
	}
}

// lookup attempts to lookup a directory entry returning a new FSHandle.
// must be called with mtx UNLOCKED
// returns with mtx UNLOCKED
func (i *fsInode) lookup(ctx context.Context, name string) (*FSHandle, error) {
	// lock i
	rel, lerr := i.rmtx.Lock(ctx, true)
	if lerr != nil {
		return nil, lerr
	}

	// create or look up the child inode
	var nref *FSHandle
	var childReady bool

	childInode, insertIdx := i.findChildInode(name, false)

	var wasReleased bool
	if childInode != nil && childInode.checkReleased() {
		childInode, wasReleased = nil, true
	}
	if childInode != nil {
		// lock the child inode
		childRel, err := childInode.rmtx.Lock(ctx, true)
		if err != nil {
			rel()
			return nil, err
		}

		// add reference to child inode & check if it was released again
		nref, err = childInode.addReferenceLocked(true)
		if err != nil {
			// the only error addReferenceLocked can return is ErrReleased
			childInode, wasReleased = nil, true
		} else {
			// check if the child is already resolved or not
			childReady = childInode.fsOps != nil && !childInode.fsOps.CheckReleased()
		}

		// release child inode
		childRel()
	}

	// create the new child inode if necessary
	if wasReleased || childInode == nil {
		// create the new child inode
		childInode = newFsInode(i, name, nil)

		// no need to lock the child inode yet since we are the first to use it.
		nref, _ = childInode.addReferenceLocked(false)
		if wasReleased {
			// insert at the old released index
			i.children[insertIdx] = childInode
		} else {
			// child inode not found, insert at insertidx.
			i.children = slices.Insert(i.children, insertIdx, childInode)
		}
	}

	// release lock on i
	rel()

	// if the child was already resolved, return now.
	if childReady {
		return nref, nil
	}

	// wait until the child inode ops are resolved
	// this verifies that the inode actually exists.
	if err := childInode.accessInode(ctx, nil); err != nil {
		nref.Release()
		return nil, err
	}

	// the reference is valid and the inode was resolved
	return nref, nil
}

// findChildInode looks for an existing non-released child by name.
// returns nil, insertIdx, error
func (i *fsInode) findChildInode(name string, checkReleased bool) (*fsInode, int) {
	idx := sort.Search(len(i.children), func(ix int) bool {
		return i.children[ix].name >= name
	})
	if idx >= len(i.children) || i.children[idx].name != name {
		// not found
		return nil, idx
	}
	child := i.children[idx]
	if checkReleased && child.checkReleased() {
		// child already released.
		i.removeChildInodeAtIdx(idx)
		return nil, idx
	}
	return child, idx
}

// removeChildInodeAtIdx removes a child from the children array at an index.
func (i *fsInode) removeChildInodeAtIdx(idx int) {
	// remove child w/o affecting sorting order
	// delete (from slicetricks)
	i.children = append(i.children[:idx], i.children[idx+1:]...)
}

// removeRefLocked removes a ref from the refs list.
// if the refs list is now empty, calls releaseIfNecessaryLocked.
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
		_ = i.releaseIfNecessaryLocked()
	}
}

// releaseIfNecessaryLocked releases this inode if it has no refs and no children.
// returns if the node was released
func (i *fsInode) releaseIfNecessaryLocked() bool {
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
// caller must hold the lock
// caller must ensure all children are released first
// if err is set, sets the fs inode error to err
func (i *fsInode) releaseLocked(err error) {
	if i.isReleased.Swap(true) {
		return
	}
	if err != nil {
		i.relErr = err
	}
	i.refs = nil
	i.children = nil
	i.fsOps = nil
	i.fsWait = nil

	// release all fs cursors
	cursors := i.fsCursors
	i.fsCursors = nil
	for _, v := range slices.Backward(cursors) {
		v.Release()
	}

	// call release callbacks in new goroutines
	for {
		cb := i.relCbs.Pop()
		if cb == nil {
			break
		}
		go cb()
	}
}

// releaseWithChildrenLocked releases this inode and all child inodes
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

	// TODO: do we need to lock the child nodes as well?

	// release in the correct order (bottom-up)
	for len(toRelease) != 0 {
		next := toRelease[len(toRelease)-1]
		next.releaseLocked(err)
		toRelease = toRelease[:len(toRelease)-1]
	}

	// finally release this node
	i.releaseLocked(err)
}
