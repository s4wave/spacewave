package unixfs

import (
	"context"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
)

// if the callback returns ErrReleased, the operation will be retried
// caller must NOT hold rmtx
// cb is called with rmtx UNLOCKED!
func (i *fsInode) accessInode(ctx context.Context, cb accessInodeCb) error {
	var lastErr error
	handleErr := func(err error) error {
		// if this isn't a ErrReleased, return it immediately.
		if err != unixfs_errors.ErrReleased {
			return err
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			err = context.Canceled
		} else if i.checkReleased() && i.relErr != nil {
			err = i.relErr
		} else if i.parent != nil && i.parent.checkReleased() && i.parent.relErr != nil {
			err = i.parent.relErr
		}

		// if this isn't a ErrReleased, return it immediately.
		if err != unixfs_errors.ErrReleased {
			return err
		}

		// return nil to indicate retry
		lastErr = err
		return nil
	}

	for tries := 0; tries < fsInodeTries; tries++ {
		isRel, relErr := i.checkReleasedWithErr()
		if isRel {
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
// caller must NOT hold rmtx
// returns with rmtx UNLOCKED
// may return errors
func (i *fsInode) resolveOps(ctx context.Context) (FSCursor, FSCursorOps, error) {
	rel, err := i.rmtx.Lock(ctx, true)
	if err != nil {
		return nil, nil, err
	}
	// note: it's ok to call rel() multiple times.
	defer rel()

	// check current ops object
	if fsOps := i.fsOps; fsOps != nil {
		if !fsOps.CheckReleased() {
			cursor := i.fsCursors[len(i.fsCursors)-1]
			return cursor, fsOps, nil
		} else {
			i.fsOps = nil
		}
	}

	// we need to fetch fsOps.
	// check if there is an existing fetch op and subscribe to it if so
	fsWait := i.fsWait
	if fsWait != nil {
		// Check if fsWait was already closed (finished).
		select {
		case <-fsWait:
			fsWait = nil
		default:
		}
	}

	waiting := fsWait != nil
	if waiting {
		// unlock rmtx for now
		rel()

		// wait for other resolve process to complete
		// then return the "try again" signal (nil, nil)
		select {
		case <-ctx.Done():
			return nil, nil, context.Canceled
		case <-fsWait:
			return nil, nil, nil
		}
	}

	// we will perform the lookup
	fsWait = make(chan struct{})
	i.fsWait = fsWait
	// expects mtx to be locked on entry & released on exit.
	i.resolveOpsRoutineLocked(ctx, fsWait, rel)

	// note: rmtx is unlocked by resolveOpsRoutineLocked
	// return context.Canceled if context was canceled
	if ctx.Err() != nil {
		return nil, nil, context.Canceled
	}

	// return try-again signal otherwise
	return nil, nil, nil
}

// resolveOpsRoutine is a separate goroutine to resolve the fsOps.
// rmtx must be held by caller
// returns with rmtx UNLOCKED
// should be called only by resolveOps
func (i *fsInode) resolveOpsRoutineLocked(ctx context.Context, fsWait chan struct{}, rel func()) {
	parent := i.parent
	iname := i.name

	// when returning indicate we finished our work
	defer close(fsWait)

	// note: rmtx is already locked by the caller.
	// if the fsOps already exists and is not released, return.
	if fsOps := i.fsOps; fsOps != nil {
		if fsOps.CheckReleased() {
			i.fsOps = nil
		} else {
			// job done: i.fsOps is set and not released
			return
		}
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
	cursorStack = slices.Clone(cursorStack)

	// unlock rmtx
	rel()

	// we may need to try this several times.
	var fsOps FSCursorOps
	var err error
	for {
		// if there are 0 cursors remaining use lookup to get the first one
		if len(cursorStack) == 0 {
			// wait for the parent to be resolved
			if parent != nil {
				err = parent.accessInode(ctx, func(cursor FSCursor, ops FSCursorOps) error {
					// lookup this dirent
					iCursor, err := ops.Lookup(ctx, iname)
					if err == nil && iCursor == nil {
						err = unixfs_errors.ErrNotExist
					}
					if err != nil {
						return err
					}

					// append without locking rmtx since nobody but us can touch cursorStack.
					cursorStack = append(cursorStack, iCursor)
					return nil
				})
			} else {
				// this cursor is released and there's no parent.
				// return ErrInodeUnresolvable
				err = errors.Wrap(unixfs_errors.ErrInodeUnresolvable, "no parent inode")
			}
			if err != nil {
				// error fetching parent cursors.
				// lock rmtx and release this + all children
				rel, relErr := i.rmtx.Lock(ctx, true)
				if relErr == nil {
					i.releaseWithChildrenLocked(err)
					i.fsWait = nil
					rel()
				}
				return
			}
		}

		// resolve the proxies as needed
		for len(cursorStack) != 0 {
			// check if the inode itself was released
			if i.checkReleased() {
				return
			}

			next := cursorStack[len(cursorStack)-1]
			var pcursor FSCursor
			pcursor, err = next.GetProxyCursor(ctx)
			if err != nil {
				if err != unixfs_errors.ErrReleased {
					break
				}
				cursorStack[len(cursorStack)-1] = nil
				cursorStack = cursorStack[:len(cursorStack)-1]
				continue
			}
			if pcursor != nil {
				cursorStack = append(cursorStack, pcursor)
				continue
			}

			// no more proxies: return the ops
			fsOps, err = next.GetCursorOps(ctx)
			if err == nil && fsOps == nil {
				err = unixfs_errors.ErrNotExist
			}
			if err != nil {
				if err != unixfs_errors.ErrReleased {
					break
				}

				// note: don't set the elem to nil here since we don't lock rmtx.
				cursorStack[len(cursorStack)-1] = nil
				cursorStack = cursorStack[:len(cursorStack)-1]
				continue
			}

			// done
			break
		}

		// if fsOps is set and not released, we are done.
		if fsOps != nil && !fsOps.CheckReleased() {
			break
		}

		// otherwise try again
		fsOps = nil
	}

	// err is set if anything failed.
	rel, relErr := i.rmtx.Lock(ctx, true)
	if relErr != nil {
		// make sure we don't leak cursors
		for i := len(cursorStack) - 1; i >= 0; i-- {
			cursorStack[i].Release()
		}
		return
	}
	defer rel()

	i.fsCursors, i.fsOps, i.fsWait = cursorStack, fsOps, nil
	if err != nil {
		i.releaseWithChildrenLocked(err)
	}
}
