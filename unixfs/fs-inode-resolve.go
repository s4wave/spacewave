package unixfs

import (
	"context"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// if the callback returns ErrReleased, the operation will be retried
// caller must not hold waitSema
func (i *fsInode) accessInode(ctx context.Context, cb accessInodeCb) error {
	var lastErr error
	handleErr := func(err error) error {
		// if this isn't a ErrReleased, return it immediately.
		if err != unixfs_errors.ErrReleased {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if i.f.CheckReleased() || i.checkReleased() {
			_, relErr := i.checkReleasedWithErr()
			return relErr
		}
		if i.parent != nil && i.parent.checkReleased() {
			// this node will be released as well when parent is released.
			_, relErr := i.parent.checkReleasedWithErr()
			return relErr
		}
		if i.fsOps != nil && i.fsOps.CheckReleased() {
			i.fsOps = nil
		}

		lastErr = err
		return nil
	}

	for tries := 0; tries < fsInodeTries; tries++ {
		isRel, relErr := i.checkReleasedWithErr()
		if i.f.CheckReleased() || isRel {
			if relErr != nil {
				return relErr
			}
			return unixfs_errors.ErrReleased
		}

		// resolve the ops object
		opsCursor, ops, err := i.resolveOps(ctx)
		if err == nil && ops == nil {
			// try-again case
			continue
		}
		if err != nil {
			if herr := handleErr(err); herr != nil {
				return herr
			}
			continue
		}

		err = cb(opsCursor, ops)
		if err != nil {
			if err == unixfs_errors.ErrReleased && !ops.CheckReleased() {
				return err
			}
			if herr := handleErr(err); herr != nil {
				return herr
			}
			continue
		}
		return nil
	}

	if lastErr != nil && lastErr != unixfs_errors.ErrReleased {
		return lastErr
	}

	return unixfs_errors.ErrInodeUnresolvable
}

// resolveOps resolves the inode operations.
// low-level op used by accessInode, use accessInode instead.
// caller must NOT hold waitSema
// waitSema may be released temporarily
// may return errors
func (i *fsInode) resolveOps(ctx context.Context) (FSCursor, FSCursorOps, error) {
	if err := i.f.waitSema.Acquire(ctx, 1); err != nil {
		return nil, nil, err
	}

	// check current ops object
	fsOps := i.fsOps
	if fsOps != nil {
		if fsOps.CheckReleased() {
			fsOps = nil
			i.fsOps = nil
			i.checkCursorsLocked()
		} else {
			i.f.waitSema.Release(1)
			return i.fsCursors[len(i.fsCursors)-1], fsOps, nil
		}
	}

	// we need to fetch fsOps.
	// check if there is an existing fetch op and subscribe to it if so
	fsOpsWait := i.fsOpsWait
	if fsOpsWait != nil {
		select {
		case <-fsOpsWait:
			fsOpsWait = nil
		default:
		}
	}
	waiting := fsOpsWait != nil
	if !waiting {
		// we will perform the lookup
		fsOpsWait = make(chan struct{})
		i.fsOpsWait = fsOpsWait
	}

	// unlock waitSema for now
	i.f.waitSema.Release(1)

	// we are the routine that will perform the fetch.
	if !waiting {
		// go i.resolveOpsRoutine(fsOpsWait)
		i.resolveOpsRoutine(fsOpsWait)
	}

	// if waiting, wait for other resolve process to complete
	// then return the "try again" signal (nil, nil)
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-fsOpsWait:
		return nil, nil, nil
	}
}

// resolveOpsRoutine is a separate goroutine to resolve the fsOps.
// waitSema must NOT be held
// should be called only by resolveOps
func (i *fsInode) resolveOpsRoutine(fsOpsWait chan struct{}) {
	parent := i.parent
	iname := i.name
	ctx := i.f.ctx

	// TODO: we should try to re-use the existing fsCursors stack on the inode.
	// the below code will build a new cursors stack and then release the old.
	// instead, starting with the old fsCursors set:
	// - from last -> first, remove any cursors that are released
	// - if there are 0 cursors use the below logic to get the first one via lookup
	// - perform the existing code below to call GetProxyCursor

	// when returning indicate we finished our work
	defer close(fsOpsWait)

	// if the fsOps already exists and is not released, return.
	if i.fsOps != nil {
		if !i.fsOps.CheckReleased() {
			return
		}
		i.fsOps = nil
	}

	// remove any released cursors from last -> first
	cursorStack := i.fsCursors
	for i := len(cursorStack) - 1; i >= 0; i-- {
		prevCursor := cursorStack[i]
		if prevCursor.CheckReleased() {
			cursorStack[i] = nil
			cursorStack = cursorStack[:i]
		}
	}
	i.fsCursors = cursorStack

	// if there are 0 cursors remaining use lookup to get the first one
	if len(cursorStack) == 0 {
		// wait for the parent to be resolved
		if parent != nil {
			err := parent.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
				// lookup this dirent
				iCursor, err := ops.Lookup(ctx, iname)
				if err == nil && iCursor == nil {
					err = unixfs_errors.ErrNotExist
				}
				if err != nil {
					return err
				}
				cursorStack = append(cursorStack, iCursor)
				return nil
			})
			if err != nil {
				// error fetching parent cursors.
				// lock waitSema and release this + all children
				if err != context.Canceled && err != unixfs_errors.ErrNotExist && i.f.le != nil {
					i.f.le.WithError(err).Warn("fs: error fetching parent cursor")
				}
				i.release(err)
				return
			}
		} else {
			// note: immutable
			rootFSCursor := i.f.rootFSCursor
			if rootFSCursor.CheckReleased() {
				if !i.checkReleased() {
					if i.f.le != nil {
						i.f.le.Warn("fs: cannot resolve, root fs cursor is released")
					}
					i.release(nil)
				}
				return
			}
			cursorStack = append(cursorStack, rootFSCursor)
		}
	}

	// resolve the proxies as needed
	var fsOps FSCursorOps
	failCleanup := func(withErr error) {
		for i := len(cursorStack) - 1; i >= 0; i-- {
			cursorStack[i].Release()
		}
		cursorStack = nil
		i.release(withErr)
	}
	for len(cursorStack) != 0 {
		if i.checkReleased() {
			failCleanup(nil)
			return
		}

		next := cursorStack[len(cursorStack)-1]
		pcursor, err := next.GetProxyCursor(ctx)
		if err != nil {
			if err == unixfs_errors.ErrReleased {
				cursorStack[len(cursorStack)-1] = nil
				cursorStack = cursorStack[:len(cursorStack)-1]
				continue
			}

			// error, release this + all children
			if err != context.Canceled && i.f.le != nil {
				i.f.le.WithError(err).Warn("fs: error getting proxy cursor")
			}
			failCleanup(err)
			return
		}
		if pcursor != nil {
			cursorStack = append(cursorStack, pcursor)
			continue
		}

		// no more proxies: return the ops
		fsOps, err = next.GetFSCursorOps(ctx)
		if err == nil && fsOps == nil {
			err = unixfs_errors.ErrNotExist
		}
		if err != nil {
			if err == unixfs_errors.ErrReleased {
				cursorStack[len(cursorStack)-1] = nil
				cursorStack = cursorStack[:len(cursorStack)-1]
				continue
			}

			// error getting the fs cursor ops.
			if err != context.Canceled && i.f.le != nil {
				i.f.le.WithError(err).Warn("fs: error fetching cursor ops")
			}
			failCleanup(err)
			return
		}
		break
	}

	if fsOps == nil {
		if i.f.le != nil {
			i.f.le.Warn("fs: failed to resolve ops: all parent cursors were released")
		}
		failCleanup(nil)
		return
	}

	// fsOps is now set to the latest ops
	// cursorStack is set to the next fsCursors value.
	if err := i.f.waitSema.Acquire(i.f.ctx, 1); err != nil {
		failCleanup(err)
		return
	}
	if i.checkReleased() {
		i.f.waitSema.Release(1)
		failCleanup(nil)
		return
	}
	// set new cursor stack
	i.fsCursors = cursorStack
	i.fsOps = fsOps
	// done
	i.f.waitSema.Release(1)
}
