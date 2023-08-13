package unixfs

import (
	"context"
	"sort"
	"sync/atomic"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/util/cqueue"
	"github.com/aperturerobotics/util/csync"
	"golang.org/x/exp/slices"
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
// caller must have checked if the inode is released and locked waitSema
func (i *fsInode) addReferenceLocked() (*FSHandle, error) {
	_, relErr := i.checkReleasedWithErr()
	if relErr != nil {
		return nil, relErr
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
	for i := len(toRelease) - 1; i >= 0; i-- {
		next := toRelease[i]
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
	for i := len(toRelease) - 1; i >= 0; i-- {
		next := toRelease[i]
		toRelease = toRelease[:i]

		next.fsOps = nil
		for i := len(next.fsCursors) - 1; i >= 0; i-- {
			next.fsCursors[i].Release()
		}
		next.fsCursors = nil
	}
}

// lookup attempts to lookup a directory entry returning a new FSHandle.
// must be called with mtx UNLOCKED
// returns with mtx UNLOCKED
func (i *fsInode) lookup(ctx context.Context, name string) (*FSHandle, error) {
	rel, err := i.rmtx.Lock(ctx, true)
	if err != nil {
		return nil, err
	}

	// fast path: inode child already exists
	childInode, _ := i.findChildInode(name, true)
	if childInode != nil {
		nref, err := childInode.addReferenceLocked()
		rel()
		return nref, err
	}

	// fast-ish path: ops is already resolved, access it
	ops := i.fsOps
	if ops != nil && ops.CheckReleased() {
		ops = nil
		i.fsOps = nil
	}
	rel()

	var lcursor FSCursor
	accessLookup := func(_ FSCursor, ops FSCursorOps) error {
		var err error
		lcursor, err = ops.Lookup(ctx, name)
		if err != nil {
			lcursor = nil
		}
		return err
	}

	if ops != nil {
		// ignore error if this doesn't work first try.
		_ = accessLookup(nil, ops)
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
	rel, err = i.rmtx.Lock(ctx, true)
	if err != nil {
		lcursor.Release()
		return nil, err
	}

	// nChild is the new child inode
	nChild := newFsInode(i, name, []FSCursor{lcursor})

	// get insert idx
	childInode, insertIdx := i.findChildInode(name, false)
	if childInode != nil {
		if childInode.checkReleased() {
			// insert new inode at index
			i.children[insertIdx] = nChild
		} else {
			// race: inode was resolved while we were working.
			// throw out our cop and use theirs.
			nChild.releaseWithChildrenLocked(nil)
			nref, err := childInode.addReferenceLocked()
			rel()
			return nref, err
		}
	}

	// child inode not found, insert at insertidx.
	i.children = slices.Insert(i.children, insertIdx, nChild)

	nref, err := nChild.addReferenceLocked()
	rel()
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
			i.removeChildInodeAtIdx(idx)
		} else {
			return child, idx
		}
	}
	return nil, idx
}

// removeChildInodeAtIdx removes a child from the children array at an index.
func (i *fsInode) removeChildInodeAtIdx(idx int) {
	// remove child w/o affecting sorting order
	// delete (from slicetricks)
	i.children = append(i.children[:idx], i.children[idx+1:]...)
}

// removeRefLocked removes a ref from the refs list.
// if the refs list is now empty, calls releaseIfNecessaryLocked.
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
		_ = i.releaseIfNecessaryLocked()
	}
}

// releaseIfNecessaryLocked releases this inode if it has no refs and no children.
// caller must hold fs waitSema
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
	for ix := len(cursors) - 1; ix >= 0; ix-- {
		cursors[ix].Release()
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
