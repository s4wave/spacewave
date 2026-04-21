package unixfs_rpc_server

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// FSCursorService exposes an FSCursor and CursorOps tree via RPC.
//
// The server and client track FSCursor and CursorOps handles via integer IDs.
// The handle IDs start at 1, a zero ID indicates nil (empty).
type FSCursorService struct {
	// mtx guards below fields
	// note: mtx is only ever locked for very short periods of time.
	// long-lived operations (like GetProxyCursor) are taken while mtx is unlocked.
	mtx sync.Mutex

	// handleIDCtr is a counter for new handle ids.
	// add 1 to it and use the added value for the next id.
	handleIDCtr uint64

	// bcastCh is closed whenever any of the below fields change.
	// nil until at least one waiter is listening
	bcastCh chan struct{}
	// handleIDToCursor maps the handle ID to the FSCursor.
	handleIDToCursor map[uint64]*localFSCursor
	// clients contains the map of ongoing FSCursorClient sessions.
	clients map[uint64]*fsCursorClient
	// handleIDToOps converts a handle ID to a FSCursorOps object.
	handleIDToOps map[uint64]unixfs.FSCursorOps
}

// localFSCursor contains a local FSCursor we expose to the remote.
// manages reference count tracking and event routing for interested clients.
// all fields are guarded by FSCursorService.mtx
type localFSCursor struct {
	// released contains a multi-state of the released/init status of localFSCursor
	released atomic.Int32 // 0: not initialized yet, 1: initialized (send event), 2: released
	// parent is the identifier of the parent cursor, zero if root.
	parent uint64
	// name is the name of the entry traversed from parent via Lookup
	// set only if parent != 0
	name string
	// clients contains the list of client IDs referencing this cursor location
	// these clients will receive notifications when the cursor changes.
	clients []uint64
	// cursor contains the FSCursor at this location.
	// this will be nil if the cursor has not been resolved yet.
	cursor unixfs.FSCursor
	// proxyHandleID is the handle identifier for the proxy cursor.
	// possible values:
	//  - nil: not resolved & no routine is attempting to resolve.
	//  - -1: GetProxyCursor returned nil, return nil for GetProxyCursor.
	//  - 0: a routine is attempting to resolve, wait for it.
	//  - >0: the handle was resolved.
	proxyHandleID *int64
	// opsHandleID is the handle identifier for the ops.
	// possible values:
	//  - nil: not resolved & no routine is attempting to resolve.
	//  - 0: a routine is attempting to resolve, wait for it.
	//  - >0: the handle was resolved.
	opsHandleID *uint64
	// children contains any child FSCursor nodes obtained from Lookup on the Ops.
	children map[string]uint64
}

// release conditionally calls release on the fsCursor.
func (l *localFSCursor) release() {
	if l.released.Swap(2) != 2 && l.cursor != nil {
		// Release in a separate routine to avoid deadlocks.
		go l.cursor.Release()
	}
}

// fsCursorClient contains information about an ongoing FSCursorClient request.
type fsCursorClient struct {
	// released indicates the client was released
	released bool
	// txQueue is a queue of messages to transmit to the client.
	txQueue []*unixfs_rpc.FSCursorChange
	// cursors is the list of cursor IDs that the client is interested in.
	cursors []uint64
}

// enqueueChange enqueues a change for transmission.
func (c *fsCursorClient) enqueueChange(ch *unixfs_rpc.FSCursorChange) {
	c.txQueue = append(c.txQueue, ch)
}

// NewFSCursorService constructs a FSCursorService from a FSCursor.
//
// NOTE: The FSCursor should never be released as long as the rpc service is available.
// Wrap your FSCursor with a unixfs.FSCursorGetter if you need to auto re-construct the root.
func NewFSCursorService(rootCursor unixfs.FSCursor) *FSCursorService {
	return &FSCursorService{
		handleIDCtr: 2, // root cursor is id #1
		handleIDToCursor: map[uint64]*localFSCursor{
			1: {cursor: rootCursor},
		},
		clients:       make(map[uint64]*fsCursorClient),
		handleIDToOps: make(map[uint64]unixfs.FSCursorOps),
	}
}

// NewFSCursorServiceWithHandle constructs a FSCursorService with a FSHandle.
//
// Constructs a FSCursor from the FSHandle when the service is accessed.
func NewFSCursorServiceWithHandle(rootHandle *unixfs.FSHandle) *FSCursorService {
	return NewFSCursorService(unixfs.NewFSCursorGetterWithHandle(rootHandle))
}

// lookupFSCursorLocked looks up the given fs cursor handle id while locked.
// if the cursor is released, drops the cursor from the set & transmits release notifications.
// checks if the cursor was released before returning
// returns nil if the cursor was not found
func (f *FSCursorService) lookupFSCursorLocked(cursorHandleID uint64) *localFSCursor {
	cursor := f.handleIDToCursor[cursorHandleID]
	if cursor == nil {
		return nil
	}
	if cursor.cursor != nil && cursor.cursor.CheckReleased() {
		f.releaseFSCursorLocked(cursorHandleID, cursor, nil)
		return nil
	}
	return cursor
}

// releaseFSCursorLocked releases the fs cursor and enqueues a notification to clients.
// if the passed cursor is nil, we will look it up by cursorHandleID
// if ch is nil, we will construct a change object.
func (f *FSCursorService) releaseFSCursorLocked(
	cursorHandleID uint64,
	cursor *localFSCursor,
	ch *unixfs_rpc.FSCursorChange,
) {
	// lookup the cursor if we didn't pass it
	if cursor == nil {
		cursor = f.handleIDToCursor[cursorHandleID]
		if cursor == nil {
			return
		}
	}

	// release the cursor if it hasn't been already
	cursor.release()

	// drop the cursor ops, if applicable.
	if cursor.opsHandleID != nil {
		opsHandleID := *cursor.opsHandleID
		if opsHandleID != 0 {
			delete(f.handleIDToOps, opsHandleID)
		}
	}

	// drop the cursor
	delete(f.handleIDToCursor, cursorHandleID)

	// transmit the released event if there are any clients
	if len(cursor.clients) != 0 {
		// build the change if we didn't pre-allocate it
		if ch == nil {
			ch = &unixfs_rpc.FSCursorChange{
				CursorHandleId: cursorHandleID,
				Released:       true,
			}
		}

		// enqueue the change now so that the child will be released before the parent
		f.enqueueCursorChangeLocked(cursor, ch)
	}

	// drop the cursor from the parent
	parentCursorID := cursor.parent
	if parentCursorID != 0 {
		parentCursor := f.lookupFSCursorLocked(parentCursorID)
		if parentCursor != nil {
			cursorName := cursor.name
			if cursorName != "" {
				if parentCursor.children[cursorName] == cursorHandleID {
					delete(parentCursor.children, cursorName)
					f.maybeReleaseFSCursorLocked(parentCursorID, parentCursor)
				}
			} else if parentCursor.proxyHandleID != nil && *parentCursor.proxyHandleID == int64(cursorHandleID) { //nolint:gosec
				parentCursor.proxyHandleID = nil
			}
		}
	}

	// broadcast change
	f.broadcastLocked()
}

// enqueueCursorChangeLocked enqueues a cursor change to all clients of the cursor.
// expects mtx is locked by caller
func (f *FSCursorService) enqueueCursorChangeLocked(cursor *localFSCursor, ch *unixfs_rpc.FSCursorChange) {
	// enqueue the change to all interested clients
	var dirty bool
	for _, clientID := range cursor.clients {
		client, clientOk := f.clients[clientID]
		if clientOk {
			dirty = true
			client.enqueueChange(ch)
		}
	}

	// broadcast that the change was made
	if dirty {
		f.broadcastLocked()
	}
}

// lookupCursorOpsLocked gets the CursorOps for the given ops handle id.
// expects mtx to be locked
// returns the ops or an error (ErrReleased)
func (f *FSCursorService) lookupCursorOpsLocked(opsHandleID uint64) (unixfs.FSCursorOps, error) {
	ops, opsExists := f.handleIDToOps[opsHandleID]
	if !opsExists {
		return nil, unixfs_errors.ErrReleased
	}
	// TODO: check if this is needed or not
	if ops.CheckReleased() {
		delete(f.handleIDToOps, opsHandleID)
		return nil, unixfs_errors.ErrReleased
	}
	return ops, nil
}

// accessCursorOps locks the mtx and calls a callback to access the CursorOps.
// expects mtx to NOT be locked
// translates the returned error to a UnixFSError, if any
func (f *FSCursorService) accessCursorOps(opsHandleID uint64, cb func(ops unixfs.FSCursorOps) error) *unixfs_errors.UnixFSError {
	if opsHandleID == 0 {
		return unixfs_errors.NewUnixFSError(unixfs_rpc.ErrHandleIDEmpty)
	}
	f.mtx.Lock()
	ops, err := f.lookupCursorOpsLocked(opsHandleID)
	f.mtx.Unlock()
	if err == nil {
		err = cb(ops)
	}
	return unixfs_errors.NewUnixFSError(err)
}

// resolveFSCursorProxy gets the FSCursor proxy for a parent handle ID.
// expects mtx to NOT be locked
// returns the cursor or an error
// returns nil, 0, nil if GetProxyCursor returned nil.
func (f *FSCursorService) resolveFSCursorProxy(ctx context.Context, parentCursorHandleID, clientID uint64) (unixfs.FSCursor, uint64, error) {
	if parentCursorHandleID == 0 {
		return nil, 0, unixfs_rpc.ErrHandleIDEmpty
	}
	if clientID == 0 {
		return nil, 0, unixfs_rpc.ErrClientIDEmpty
	}

	// we may need to wait some time for the cursor to be resolved by a different routine.
	var wait <-chan struct{}
	for {
		if wait != nil {
			select {
			case <-ctx.Done():
				return nil, 0, context.Canceled
			case <-wait:
			}
		}

		f.mtx.Lock()

		// check the client is still valid.
		clientObj := f.clients[clientID]
		if clientObj == nil || clientObj.released {
			f.mtx.Unlock()
			return nil, 0, unixfs_errors.ErrReleased
		}

		// check the parent cursor is still valid.
		parentCursor := f.lookupFSCursorLocked(parentCursorHandleID)
		if parentCursor == nil {
			f.mtx.Unlock()
			return nil, 0, unixfs_errors.ErrReleased
		}

		// we have to wait for the cursor to be resolved.
		fsCursor := parentCursor.cursor
		if fsCursor == nil {
			wait = f.getWaitChLocked()
			f.mtx.Unlock()
			continue
		}

		// if the pointer is set, another routine has/is resolving it.
		if parentCursor.proxyHandleID != nil {
			proxyHandleID := *parentCursor.proxyHandleID
			if proxyHandleID == 0 {
				// another routine is resolving the proxy cursor.
				// wait for that routine to finish.
				wait = f.getWaitChLocked()
				f.mtx.Unlock()
				continue
			} else if proxyHandleID == -1 {
				// GetProxyCursor previously returned nil, return nil now.
				f.mtx.Unlock()
				return nil, 0, nil
			}

			// proxyHandleID contains the ID of the proxy cursor handle.
			proxyCursor := f.handleIDToCursor[uint64(proxyHandleID)] //nolint:gosec
			if proxyCursor == nil {
				// the proxy cursor was released somewhere else.
				parentCursor.proxyHandleID = nil
			} else if proxyCursor.cursor != nil {
				if proxyCursor.cursor.CheckReleased() {
					// the cursor was already released, drop it.
					f.releaseFSCursorLocked(uint64(proxyHandleID), proxyCursor, nil) //nolint:gosec
					parentCursor.proxyHandleID = nil
				} else {
					// the proxy cursor is good. add our client & return it
					if !slices.Contains(parentCursor.clients, clientID) {
						parentCursor.clients = append(parentCursor.clients, clientID)
						clientObj.cursors = append(clientObj.cursors, uint64(proxyHandleID)) //nolint:gosec
					}
					f.mtx.Unlock()
					return proxyCursor.cursor, uint64(proxyHandleID), nil //nolint:gosec
				}
			} else {
				// wait for the cursor to be resolved.
				wait = f.getWaitChLocked()
				f.mtx.Unlock()
				continue
			}
		}

		// we are the first to call resolveFSCursorProxy for this cursor
		parentCursor.proxyHandleID = new(int64)

		// call GetProxyCursor
		retHandleID := f.handleIDCtr
		f.handleIDCtr++

		var retNil bool
		var proxyFsCursor unixfs.FSCursor
		err := f.ingestFSCursorLocked(retHandleID, func() (unixfs.FSCursor, error) {
			proxyCursor, err := fsCursor.GetProxyCursor(ctx)
			retNil = err == nil && proxyCursor == nil
			proxyFsCursor = proxyCursor
			return proxyCursor, err
		}, parentCursorHandleID, "", clientID)

		// register the returned cursor (if applicable)
		if retNil {
			// mark as nil return value
			err = nil
			*parentCursor.proxyHandleID = -1
			proxyFsCursor = nil
			retHandleID = 0
		} else if err != nil {
			parentCursor.proxyHandleID = nil
		} else {
			*parentCursor.proxyHandleID = int64(retHandleID) //nolint:gosec
			if !slices.Contains(clientObj.cursors, retHandleID) {
				clientObj.cursors = append(clientObj.cursors, retHandleID)
			}
		}

		// signal to other watchers that the request is done
		f.broadcastLocked()
		f.mtx.Unlock()

		// return the result
		return proxyFsCursor, retHandleID, err
	}
}

// resolveCursorOps gets the CursorOps for a handle ID while locked.
// expects mtx to NOT be locked
// returns the ops or an error
// returns nil, ErrNotExist if GetCursorOps returned nil, nil
// returns ErrReleased if the cursor is not found
// will temporarily lock mtx while resolving the ops.
func (f *FSCursorService) resolveCursorOps(ctx context.Context, cursorHandleID uint64) (unixfs.FSCursorOps, uint64, error) {
	// we may need to wait some time for the ops to be resolved by a different routine.
	var wait <-chan struct{}
	for {
		if wait != nil {
			select {
			case <-ctx.Done():
				return nil, 0, context.Canceled
			case <-wait:
			}
		}

		// this checks that the cursor is still registered & not released.
		f.mtx.Lock()
		cursor := f.lookupFSCursorLocked(cursorHandleID)
		if cursor == nil {
			f.mtx.Unlock()
			return nil, 0, unixfs_errors.ErrReleased
		}

		// we have to wait for the cursor to be resolved.
		fsCursor := cursor.cursor
		if fsCursor == nil {
			wait = f.getWaitChLocked()
			f.mtx.Unlock()
			continue
		}

		// if the pointer is set, another routine has/is resolving ops.
		if cursor.opsHandleID != nil {
			opsHandleID := *cursor.opsHandleID
			if opsHandleID == 0 {
				// another routine is resolving ops.
				// wait for that routine to finish.
				wait = f.getWaitChLocked()
				f.mtx.Unlock()
				continue
			}

			// opsHandleID contains the ID of the operations handle.
			ops := f.handleIDToOps[opsHandleID]
			if ops != nil && !ops.CheckReleased() {
				// the ops handle is valid.
				f.mtx.Unlock()
				return ops, opsHandleID, nil
			}

			// the ops handle was released already, let's drop it and rebuild.
			delete(f.handleIDToOps, opsHandleID)
		}

		// we are the first to call resolveCursorOps for this cursor
		cursor.opsHandleID = new(uint64)
		f.mtx.Unlock()

		// call GetCursorOps
		ops, err := fsCursor.GetCursorOps(ctx)

		// mark the result
		f.mtx.Lock()
		if ops == nil && err == nil {
			// correct the err according to the comment on the func
			err = unixfs_errors.ErrNotExist
		}

		// update the ops handle id
		var opsHandleID uint64
		if err == nil {
			opsHandleID = f.handleIDCtr
			f.handleIDCtr++
			*cursor.opsHandleID = opsHandleID
			f.handleIDToOps[opsHandleID] = ops
		} else {
			cursor.opsHandleID = nil
		}

		// signal to other watchers that the request is done
		f.broadcastLocked()
		f.mtx.Unlock()

		// return the result
		return ops, opsHandleID, err
	}
}

// resolveFSCursorLookup resolves a lookup for a child of an FSCursor by name.
// expects mtx to NOT be locked
// returns the cursor or an error
// returns nil, 0, nil if GetProxyCursor returned nil.
// checks that the cursor handle id matches the ops handle id.
// returns ErrReleased if either the cursor or the ops handle id are released or mismatch.
func (f *FSCursorService) resolveFSCursorLookup(
	ctx context.Context,
	parentCursorHandleID,
	opsHandleID,
	clientID uint64,
	lookupName string,
) (unixfs.FSCursor, uint64, error) {
	if parentCursorHandleID == 0 || opsHandleID == 0 {
		return nil, 0, unixfs_rpc.ErrHandleIDEmpty
	}
	if clientID == 0 {
		return nil, 0, unixfs_rpc.ErrClientIDEmpty
	}
	if lookupName == "" {
		return nil, 0, unixfs_errors.ErrEmptyPath
	}

	// we may need to wait some time for the cursor to be resolved by a different routine.
	var wait <-chan struct{}
	for {
		if wait != nil {
			select {
			case <-ctx.Done():
				return nil, 0, context.Canceled
			case <-wait:
			}
		}

		f.mtx.Lock()

		// check the client is still valid.
		clientObj := f.clients[clientID]
		if clientObj == nil || clientObj.released {
			f.mtx.Unlock()
			return nil, 0, unixfs_errors.ErrReleased
		}

		// check that the parent cursor is still registered & not released & ops handle id matches.
		parentCursor := f.lookupFSCursorLocked(parentCursorHandleID)
		if parentCursor == nil || parentCursor.cursor == nil || parentCursor.opsHandleID == nil || *parentCursor.opsHandleID != opsHandleID {
			f.mtx.Unlock()
			return nil, 0, unixfs_errors.ErrReleased
		}

		// check if another routine is resolving or has resolved this child already.
		childCursorID, childCursorOk := parentCursor.children[lookupName]
		if childCursorOk {
			childCursor := f.handleIDToCursor[childCursorID]
			if childCursor == nil {
				delete(parentCursor.children, lookupName)
			} else if childCursor.cursor == nil {
				// another routine is resolving the cursor.
				// wait for that routine to finish.
				wait = f.getWaitChLocked()
				f.mtx.Unlock()
				continue
			} else {
				if !slices.Contains(childCursor.clients, clientID) {
					childCursor.clients = append(childCursor.clients, clientID)
					clientObj.cursors = append(clientObj.cursors, childCursorID)
				}
				retCursor := childCursor.cursor
				f.mtx.Unlock()
				return retCursor, childCursorID, nil
			}
		}

		// lookup the ops handle and make sure it's not released.
		parentCursorOps := f.handleIDToOps[opsHandleID]
		if parentCursorOps != nil && parentCursorOps.CheckReleased() {
			parentCursorOps = nil
		}
		if parentCursorOps == nil {
			f.mtx.Unlock()
			return nil, 0, unixfs_errors.ErrReleased
		}

		// we are the first to call Lookup for this cursor->name combination
		retHandleID := f.handleIDCtr
		f.handleIDCtr++
		if parentCursor.children == nil {
			parentCursor.children = make(map[string]uint64)
		}
		parentCursor.children[lookupName] = retHandleID

		// call Lookup
		var lookupFsCursor unixfs.FSCursor
		err := f.ingestFSCursorLocked(retHandleID, func() (unixfs.FSCursor, error) {
			cursor, err := parentCursorOps.Lookup(ctx, lookupName)
			if err == nil && cursor == nil {
				err = fs.ErrNotExist
			}
			lookupFsCursor = cursor
			return cursor, err
		}, parentCursorHandleID, lookupName, clientID)

		// clear the child if the request failed
		// if err != nil {
		// 	delete(parentCursor.children, lookupName)
		// } else {
		if err == nil && !slices.Contains(clientObj.cursors, retHandleID) {
			clientObj.cursors = append(clientObj.cursors, retHandleID)
		}

		// signal to other watchers that the request is done
		f.broadcastLocked()
		f.mtx.Unlock()

		// return the result
		return lookupFsCursor, retHandleID, err
	}
}

// removeFSCursorRefLocked removes a client id from a cursor.
// expects mtx is locked
// expects that the client already knows this cursor was released (does not send a event).
func (f *FSCursorService) removeFSCursorRefLocked(cursorHandleID, clientHandleID uint64, removeFromClient bool) error {
	if cursorHandleID == 0 {
		return unixfs_rpc.ErrHandleIDEmpty
	}
	if clientHandleID == 0 {
		return unixfs_rpc.ErrClientIDEmpty
	}

	if removeFromClient {
		clientObj := f.clients[clientHandleID]
		if clientObj == nil {
			return nil
		}

		clientIdx := slices.Index(clientObj.cursors, cursorHandleID)
		if clientIdx == -1 {
			return nil
		}
		clientObj.cursors = append(clientObj.cursors[:clientIdx], clientObj.cursors[clientIdx:]...)
	}

	cursor, cursorOk := f.handleIDToCursor[cursorHandleID]
	if !cursorOk {
		return nil
	}

	idx := slices.Index(cursor.clients, clientHandleID)
	if idx == -1 {
		return nil
	}

	// remove the client from the list of clients
	if len(cursor.clients) == 1 {
		cursor.clients = nil
		f.maybeReleaseFSCursorLocked(cursorHandleID, cursor)
	} else {
		cursor.clients = append(cursor.clients[:idx], cursor.clients[idx+1:]...)
	}

	return nil
}

// maybeReleaseFSCursorLocked conditionally releases the FSCursor.
// expects mtx to be locked
// if cursor is nil, we will look it up in this function.
// if no clients and (cursor is nil OR no children), release the cursor
func (f *FSCursorService) maybeReleaseFSCursorLocked(cursorHandleID uint64, cursor *localFSCursor) {
	if cursorHandleID == 0 {
		return
	}

	if cursor == nil {
		cursor = f.lookupFSCursorLocked(cursorHandleID)
		if cursor == nil {
			return
		}
	}

	if len(cursor.clients) == 0 && len(cursor.children) == 0 {
		f.releaseFSCursorLocked(cursorHandleID, cursor, nil)
	}
}

// resolveFSCursorLocked adds the FSCursor to the working set and resolves it.
// expects mtx to be locked when calling
// returns with mtx LOCKED.
// calls resolveFSCursor while mtx is UNLOCKED.
// will unlock the mtx while resolving the cursor or ErrReleased.
// returns the handle ID for the new cursor.
// parent should be the parent fs cursor id
// name is set if the cursor was derived from calling Lookup.
// clientID should be set to the id of the client who allocated this cursor.
func (f *FSCursorService) ingestFSCursorLocked(
	fsCursorID uint64,
	resolveFSCursor func() (unixfs.FSCursor, error),
	parent uint64,
	name string,
	clientID uint64,
) error {
	nextFSCursor := &localFSCursor{
		parent:  parent,
		name:    name,
		clients: []uint64{clientID},
	}
	if name != "" {
		// note: we expect that the caller already updated the parent->children map.
		// otherwise defer registering the cursor until later.
		f.handleIDToCursor[fsCursorID] = nextFSCursor
		f.broadcastLocked()
	}
	f.mtx.Unlock()

	// resolve the fsCursor
	fsCursor, err := resolveFSCursor()
	if err == nil && fsCursor == nil {
		// treat nil return value as ErrReleased.
		err = unixfs_errors.ErrReleased
	}

	if err == nil {
		// Add the change callback and detect instant release callback within the same stack.
		fsCursor.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
			// sanity check to avoid nil reference exceptions due to cursor bugs
			if ch == nil {
				return true
			}
			released := ch.Released
			if released {
				if nextFSCursor.released.Load() > 1 {
					// if we already released, return & remove handler
					return false
				}
				if ch.Released && nextFSCursor.released.Swap(2) != 1 {
					// if released == 0 we released before initialization was complete.
					// avoid a mutex contention by returning now.
					return false
				}
			} else if nextFSCursor.released.Load() == 0 {
				// if released == 0 we didn't finish init yet.
				return true
			}

			// ingestFSCursorLocked previously returned success.
			// remove the fsCursor now.
			cc := unixfs_rpc.NewFSCursorChange(fsCursorID, ch)
			f.mtx.Lock()
			if released {
				// the cursor is already released: mark it as such so we don't call Release again.
				nextFSCursor.released.Store(2)
				// release from our internal structures
				f.releaseFSCursorLocked(fsCursorID, nextFSCursor, cc)
			} else {
				f.enqueueCursorChangeLocked(nextFSCursor, cc)
			}
			f.mtx.Unlock()

			return true
		})

		// we already marked as released in the call above if != 0
		if swapped := nextFSCursor.released.CompareAndSwap(0, 1); !swapped {
			err = unixfs_errors.ErrReleased
		}
	}

	// mark the result
	f.mtx.Lock()

	// check if the parent cursor was deleted from the map already
	if err == nil {
		if _, ok := f.handleIDToCursor[parent]; !ok {
			err = unixfs_errors.ErrReleased
		}
	}

	// if we already returned an error, stop early.
	nextFSCursor.cursor = fsCursor
	if err != nil {
		// drop the list of clients first so we don't send a change notification.
		if name != "" {
			nextFSCursor.clients = nil
			f.releaseFSCursorLocked(fsCursorID, nextFSCursor, nil)
		} else {
			nextFSCursor.release()
		}
	} else if name == "" {
		// we did this already above if name != ""
		f.handleIDToCursor[fsCursorID] = nextFSCursor
	}

	return err
}

// FSCursorClient starts a new client stream allocating a client handle.
// When this RPC exits, all handles opened by the client are closed.
func (f *FSCursorService) FSCursorClient(
	req *unixfs_rpc.FSCursorClientRequest,
	strm unixfs_rpc.SRPCFSCursorService_FSCursorClientStream,
) error {
	ctx := strm.Context()

	// Add the client to the client set.
	f.mtx.Lock()
	clientHandleID := f.handleIDCtr
	f.handleIDCtr++
	clientObj := &fsCursorClient{}
	f.clients[clientHandleID] = clientObj
	eventWait := f.getWaitChLocked()
	f.mtx.Unlock()

	// Remove the client when returning.
	defer func() {
		f.mtx.Lock()
		for _, cursorID := range clientObj.cursors {
			_ = f.removeFSCursorRefLocked(cursorID, clientHandleID, false)
		}
		clientObj.cursors, clientObj.released = nil, true
		delete(f.clients, clientHandleID)
		f.mtx.Unlock()
	}()

	// Send the init message.
	if err := strm.Send(&unixfs_rpc.FSCursorClientResponse{
		Body: &unixfs_rpc.FSCursorClientResponse_Init{
			Init: &unixfs_rpc.FSClientInit{
				ClientHandleId: clientHandleID,
				CursorHandleId: 1,
			},
		},
	}); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-eventWait:
		}

		f.mtx.Lock()
		eventWait = f.getWaitChLocked()
		txQueue := clientObj.txQueue
		clientObj.txQueue = nil
		released := clientObj.released
		f.mtx.Unlock()

		if released {
			// client was already dropped.
			return unixfs_errors.ErrReleased
		}

		for _, event := range txQueue {
			if err := strm.Send(&unixfs_rpc.FSCursorClientResponse{
				Body: &unixfs_rpc.FSCursorClientResponse_CursorChange{
					CursorChange: event,
				},
			}); err != nil {
				return err
			}
		}
	}
}

// GetProxyCursor returns an FSCursor to replace an existing one, if necessary.
func (f *FSCursorService) GetProxyCursor(
	ctx context.Context,
	req *unixfs_rpc.GetProxyCursorRequest,
) (*unixfs_rpc.GetProxyCursorResponse, error) {
	var resp unixfs_rpc.GetProxyCursorResponse
	_, cursorHandleID, err := f.resolveFSCursorProxy(ctx, req.GetCursorHandleId(), req.GetClientHandleId())
	if err != nil {
		resp.UnixfsError = unixfs_errors.NewUnixFSError(err)
	} else {
		resp.CursorHandleId = cursorHandleID
	}

	return &resp, nil
}

// GetCursorOps resolves the CursorOps handle.
func (f *FSCursorService) GetCursorOps(
	ctx context.Context,
	req *unixfs_rpc.GetCursorOpsRequest,
) (*unixfs_rpc.GetCursorOpsResponse, error) {
	cursorHandleID := req.GetCursorHandleId()
	if cursorHandleID == 0 {
		return nil, unixfs_rpc.ErrHandleIDEmpty
	}

	var resp unixfs_rpc.GetCursorOpsResponse
	ops, opsHandleID, err := f.resolveCursorOps(ctx, cursorHandleID)
	if ops == nil && err == nil {
		err = unixfs_errors.ErrNotExist
	}
	if err != nil {
		resp.UnixfsError = unixfs_errors.NewUnixFSError(err)
	} else {
		resp.Name = ops.GetName()
		resp.NodeType = unixfs_block.FSCursorNodeTypeToNodeType(ops)
		resp.OpsHandleId = opsHandleID
	}

	return &resp, nil
}

// ReleaseFSCursor releases an FSCursor handle.
// This is a Fire and Forget RPC which will return instantly.
func (f *FSCursorService) ReleaseFSCursor(
	ctx context.Context,
	req *unixfs_rpc.ReleaseFSCursorRequest,
) (*unixfs_rpc.ReleaseFSCursorResponse, error) {
	f.mtx.Lock()
	err := f.removeFSCursorRefLocked(req.GetCursorHandleId(), req.GetClientHandleId(), true)
	f.mtx.Unlock()
	if err != nil {
		return nil, err
	}
	return &unixfs_rpc.ReleaseFSCursorResponse{}, nil
}

// OpsGetPermissions returns the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (f *FSCursorService) OpsGetPermissions(
	ctx context.Context,
	req *unixfs_rpc.OpsGetPermissionsRequest,
) (*unixfs_rpc.OpsGetPermissionsResponse, error) {
	var resp unixfs_rpc.OpsGetPermissionsResponse
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		perms, err := ops.GetPermissions(ctx)
		if err != nil {
			return err
		}
		resp.FileMode = uint32(perms)
		return nil
	})

	return &resp, nil
}

// OpsSetPermissions updates the permissions bits of the file mode.
func (f *FSCursorService) OpsSetPermissions(
	ctx context.Context,
	req *unixfs_rpc.OpsSetPermissionsRequest,
) (*unixfs_rpc.OpsSetPermissionsResponse, error) {
	var resp unixfs_rpc.OpsSetPermissionsResponse
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		return ops.SetPermissions(ctx, fs.FileMode(req.GetFileMode()), req.GetTimestamp().AsTime())
	})
	return &resp, nil
}

// OpsGetSize returns the size of the inode (in bytes).
func (f *FSCursorService) OpsGetSize(
	ctx context.Context,
	req *unixfs_rpc.OpsGetSizeRequest,
) (*unixfs_rpc.OpsGetSizeResponse, error) {
	var resp unixfs_rpc.OpsGetSizeResponse
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		size, err := ops.GetSize(ctx)
		resp.Size = size
		return err
	})
	return &resp, nil
}

// OpsGetModTimestamp returns the modification timestamp.
func (f *FSCursorService) OpsGetModTimestamp(
	ctx context.Context,
	req *unixfs_rpc.OpsGetModTimestampRequest,
) (*unixfs_rpc.OpsGetModTimestampResponse, error) {
	var resp unixfs_rpc.OpsGetModTimestampResponse
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		modTs, err := ops.GetModTimestamp(ctx)
		if err == nil {
			resp.ModTimestamp = timestamp.ToTimestamp(modTs)
		}
		return err
	})
	return &resp, nil
}

// OpsSetModTimestamp updates the modification timestamp of the node.
func (f *FSCursorService) OpsSetModTimestamp(
	ctx context.Context,
	req *unixfs_rpc.OpsSetModTimestampRequest,
) (*unixfs_rpc.OpsSetModTimestampResponse, error) {
	var resp unixfs_rpc.OpsSetModTimestampResponse
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		return ops.SetModTimestamp(ctx, req.GetModTimestamp().AsTime())
	})
	return &resp, nil
}

// OpsReadAt reads from a location in a File node.
func (f *FSCursorService) OpsReadAt(
	ctx context.Context,
	req *unixfs_rpc.OpsReadAtRequest,
) (*unixfs_rpc.OpsReadAtResponse, error) {
	var resp unixfs_rpc.OpsReadAtResponse
	readSize, offset := req.GetSize(), req.GetOffset()
	if readSize < 0 {
		return nil, errors.New("negative-size read not allowed")
	}
	if readSize > unixfs_rpc.ReadAtSizeLimit {
		readSize = unixfs_rpc.ReadAtSizeLimit
	}
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		// todo: use a buffer arena here?
		readBuf := make([]byte, readSize)
		readAmt, err := ops.ReadAt(ctx, offset, readBuf)
		if err != nil && err != io.EOF {
			readAmt = 0
		}
		if int(readAmt) > len(readBuf) {
			readAmt = int64(len(readBuf))
		}
		if readAmt > 0 {
			resp.Data = readBuf[:readAmt]
		}
		return err
	})
	return &resp, nil
}

// OpsGetOptimalWriteSize returns the best write size to use for the Write call.
func (f *FSCursorService) OpsGetOptimalWriteSize(
	ctx context.Context,
	req *unixfs_rpc.OpsGetOptimalWriteSizeRequest,
) (*unixfs_rpc.OpsGetOptimalWriteSizeResponse, error) {
	var resp unixfs_rpc.OpsGetOptimalWriteSizeResponse
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		optimalWriteSize, err := ops.GetOptimalWriteSize(ctx)
		resp.OptimalWriteSize = optimalWriteSize
		return err
	})
	return &resp, nil
}

// OpsWriteAt writes to a location within a File node synchronously.
func (f *FSCursorService) OpsWriteAt(
	ctx context.Context,
	req *unixfs_rpc.OpsWriteAtRequest,
) (*unixfs_rpc.OpsWriteAtResponse, error) {
	var resp unixfs_rpc.OpsWriteAtResponse
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		return ops.WriteAt(ctx, req.GetOffset(), req.GetData(), req.GetTimestamp().AsTime())
	})
	return &resp, nil
}

// OpsTruncate shrinks or extends a file to the specified size.
func (f *FSCursorService) OpsTruncate(
	ctx context.Context,
	req *unixfs_rpc.OpsTruncateRequest,
) (*unixfs_rpc.OpsTruncateResponse, error) {
	var resp unixfs_rpc.OpsTruncateResponse
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		return ops.Truncate(ctx, req.GetNsize(), req.GetTimestamp().AsTime())
	})
	return &resp, nil
}

// OpsLookup looks up a child entry in a directory.
func (f *FSCursorService) OpsLookup(
	ctx context.Context,
	req *unixfs_rpc.OpsLookupRequest,
) (*unixfs_rpc.OpsLookupResponse, error) {
	_, retCursorID, err := f.resolveFSCursorLookup(
		ctx,
		req.GetCursorHandleId(),
		req.GetOpsHandleId(),
		req.GetClientHandleId(),
		req.GetName(),
	)

	var resp unixfs_rpc.OpsLookupResponse
	if err != nil {
		resp.UnixfsError = unixfs_errors.NewUnixFSError(err)
	} else {
		resp.CursorHandleId = retCursorID
	}
	return &resp, nil
}

// OpsReaddirAll reads all directory entries in a stream.
func (f *FSCursorService) OpsReaddirAll(
	req *unixfs_rpc.OpsReaddirAllRequest,
	strm unixfs_rpc.SRPCFSCursorService_OpsReaddirAllStream,
) error {
	ctx := strm.Context()
	readErr := f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		return ops.ReaddirAll(ctx, req.GetSkip(), func(ent unixfs.FSCursorDirent) error {
			if ent == nil {
				return nil
			}
			return strm.Send(&unixfs_rpc.OpsReaddirAllResponse{
				Body: &unixfs_rpc.OpsReaddirAllResponse_Dirent{
					Dirent: unixfs_rpc.NewFSCursorDirent(ent),
				},
			})
		})
	})
	if readErr != nil {
		return strm.Send(&unixfs_rpc.OpsReaddirAllResponse{
			Body: &unixfs_rpc.OpsReaddirAllResponse_UnixfsError{
				UnixfsError: readErr,
			},
		})
	} else {
		if err := strm.Send(&unixfs_rpc.OpsReaddirAllResponse{
			Body: &unixfs_rpc.OpsReaddirAllResponse_Done{Done: true},
		}); err != nil {
			return err
		}
	}
	return strm.Close()
}

// OpsMknod creates child entries in a directory.
func (f *FSCursorService) OpsMknod(
	ctx context.Context,
	req *unixfs_rpc.OpsMknodRequest,
) (*unixfs_rpc.OpsMknodResponse, error) {
	var resp unixfs_rpc.OpsMknodResponse
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		return ops.Mknod(
			ctx,
			req.GetCheckExist(),
			req.GetNames(),
			req.GetNodeType(),
			fs.FileMode(req.GetPermissions()),
			req.GetTimestamp().AsTime(),
		)
	})
	return &resp, nil
}

// OpsSymlink creates a symbolic link from a location to a path.
func (f *FSCursorService) OpsSymlink(
	ctx context.Context,
	req *unixfs_rpc.OpsSymlinkRequest,
) (*unixfs_rpc.OpsSymlinkResponse, error) {
	var resp unixfs_rpc.OpsSymlinkResponse
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		return ops.Symlink(
			ctx,
			req.GetCheckExist(),
			req.GetName(),
			req.GetSymlink().GetTargetPath().GetNodes(),
			req.GetSymlink().GetTargetPath().GetAbsolute(),
			req.GetTimestamp().AsTime(),
		)
	})
	return &resp, nil
}

// OpsReadlink reads a symbolic link contents.
func (f *FSCursorService) OpsReadlink(
	ctx context.Context,
	req *unixfs_rpc.OpsReadlinkRequest,
) (*unixfs_rpc.OpsReadlinkResponse, error) {
	var resp unixfs_rpc.OpsReadlinkResponse
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		rp, rpAbsolute, err := ops.Readlink(
			ctx,
			req.GetName(),
		)
		if err != nil {
			return err
		}
		resp.Symlink = unixfs_block.NewFSSymlink(unixfs_block.NewFSPath(rp, rpAbsolute))
		return nil
	})
	return &resp, nil
}

// OpsCopyTo performs an optimized copy of an dirent inode to another inode.
func (f *FSCursorService) OpsCopyTo(
	ctx context.Context,
	req *unixfs_rpc.OpsCopyToRequest,
) (*unixfs_rpc.OpsCopyToResponse, error) {
	var resp unixfs_rpc.OpsCopyToResponse
	// access the source ops
	var srcOps unixfs.FSCursorOps
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(srcOpsRet unixfs.FSCursorOps) error {
		srcOps = srcOpsRet
		return nil
	})
	if resp.UnixfsError != nil {
		return &resp, nil
	}

	// access the destination ops and copy
	resp.UnixfsError = f.accessCursorOps(req.GetTargetDirOpsHandleId(), func(dstOps unixfs.FSCursorOps) error {
		// perform the operation
		done, err := srcOps.CopyTo(ctx, dstOps, req.GetTargetName(), req.GetTimestamp().AsTime())
		resp.Done = done
		return err
	})

	return &resp, nil
}

// OpsCopyFrom performs an optimized copy from another inode.
func (f *FSCursorService) OpsCopyFrom(
	ctx context.Context,
	req *unixfs_rpc.OpsCopyFromRequest,
) (*unixfs_rpc.OpsCopyFromResponse, error) {
	var resp unixfs_rpc.OpsCopyFromResponse
	// access the source ops
	var srcOps unixfs.FSCursorOps
	resp.UnixfsError = f.accessCursorOps(req.GetSrcCursorOpsHandleId(), func(srcOpsRet unixfs.FSCursorOps) error {
		srcOps = srcOpsRet
		return nil
	})
	if resp.UnixfsError != nil {
		return &resp, nil
	}

	// access the destination ops and copy
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(dstOps unixfs.FSCursorOps) error {
		// perform the operation
		done, err := dstOps.CopyFrom(ctx, req.GetName(), srcOps, req.GetTimestamp().AsTime())
		resp.Done = done
		return err
	})

	return &resp, nil
}

// OpsMoveTo performs an atomic and optimized move to another inode.
func (f *FSCursorService) OpsMoveTo(
	ctx context.Context,
	req *unixfs_rpc.OpsMoveToRequest,
) (*unixfs_rpc.OpsMoveToResponse, error) {
	var resp unixfs_rpc.OpsMoveToResponse
	// access the source ops
	var srcOps unixfs.FSCursorOps
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(srcOpsRet unixfs.FSCursorOps) error {
		srcOps = srcOpsRet
		return nil
	})
	if resp.UnixfsError != nil {
		return &resp, nil
	}
	if srcOps == nil || srcOps.CheckReleased() {
		resp.UnixfsError = unixfs_errors.NewUnixFSError(unixfs_errors.ErrReleased)
		return &resp, nil
	}

	// access the destination ops and move
	resp.UnixfsError = f.accessCursorOps(req.GetTargetDirOpsHandleId(), func(dstOps unixfs.FSCursorOps) error {
		if dstOps == nil || dstOps.CheckReleased() {
			return unixfs_errors.ErrReleased
		}

		// perform the operation
		done, err := srcOps.MoveTo(ctx, dstOps, req.GetTargetName(), req.GetTimestamp().AsTime())
		resp.Done = done
		return err
	})

	return &resp, nil
}

// OpsMoveFrom performs an atomic and optimized move from another inode.
func (f *FSCursorService) OpsMoveFrom(
	ctx context.Context,
	req *unixfs_rpc.OpsMoveFromRequest,
) (*unixfs_rpc.OpsMoveFromResponse, error) {
	var resp unixfs_rpc.OpsMoveFromResponse
	// access the source ops
	var srcOps unixfs.FSCursorOps
	resp.UnixfsError = f.accessCursorOps(req.GetSrcOpsHandleId(), func(srcOpsRet unixfs.FSCursorOps) error {
		srcOps = srcOpsRet
		return nil
	})
	if resp.UnixfsError != nil {
		return &resp, nil
	}

	// access the destination ops and copy
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(dstOps unixfs.FSCursorOps) error {
		// perform the operation
		done, err := dstOps.MoveFrom(ctx, req.GetName(), srcOps, req.GetTimestamp().AsTime())
		resp.Done = done
		return err
	})

	return &resp, nil
}

// OpsRemove deletes entries from a directory.
func (f *FSCursorService) OpsRemove(
	ctx context.Context,
	req *unixfs_rpc.OpsRemoveRequest,
) (*unixfs_rpc.OpsRemoveResponse, error) {
	var resp unixfs_rpc.OpsRemoveResponse
	resp.UnixfsError = f.accessCursorOps(req.GetOpsHandleId(), func(ops unixfs.FSCursorOps) error {
		return ops.Remove(ctx, req.GetNames(), req.GetTimestamp().AsTime())
	})
	return &resp, nil
}

// Release clears all contents of the service, releasing all cursors.
// If releaseRoot is set, the root cursor is released as well.
// Drops all clients without sending release notifications.
func (f *FSCursorService) Release(releaseRoot bool) {
	f.mtx.Lock()
	for _, client := range f.clients {
		client.released, client.cursors, client.txQueue = true, nil, nil
	}
	// f.handleIDCtr = 1
	f.clients = make(map[uint64]*fsCursorClient)
	f.handleIDToOps = make(map[uint64]unixfs.FSCursorOps)
	for id, localCursor := range f.handleIDToCursor {
		if id == 1 && !releaseRoot {
			continue
		}
		// NOTE: release calls the actual Release function in a separate goroutine to avoid deadlocks.
		localCursor.release()
		localCursor.opsHandleID, localCursor.proxyHandleID = nil, nil
		localCursor.clients = nil
		delete(f.handleIDToCursor, id)
	}
	f.broadcastLocked()
	f.mtx.Unlock()
}

// getWaitChLocked returns a channel that is closed when broadcastLocked is called.
func (f *FSCursorService) getWaitChLocked() <-chan struct{} {
	if f.bcastCh == nil {
		f.bcastCh = make(chan struct{})
	}
	return f.bcastCh
}

// broadcastLocked notifies any waiters that cursors have changed resolution state.
func (f *FSCursorService) broadcastLocked() {
	if f.bcastCh != nil {
		close(f.bcastCh)
		f.bcastCh = nil
	}
}

// TODO: if we return the same FSCursor from GetProxyCursor or the same ops from
// GetCursorOps, in other words, if we re-use objects multiple times, they will
// be treated as separate objects here.
//
// this may lead to cursors being spuriously released
// luckily, the client will just re-build the cursors in this case.
// if this is a real performance issue we can add mapping for this later.
const _ int = 0

// _ is a type assertion
var _ unixfs_rpc.SRPCFSCursorServiceServer = ((*FSCursorService)(nil))
