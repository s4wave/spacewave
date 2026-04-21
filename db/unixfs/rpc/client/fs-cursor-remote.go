package unixfs_rpc_client

import (
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
)

// remoteFSCursor implements FSCursor attached to FSCursorClient.
type remoteFSCursor struct {
	// released indicates the cursor has been released
	released atomic.Bool
	// c is the client instance
	c *FSCursorClient
	// cursorHandleID is the remote cursor handle id for requests.
	cursorHandleID uint64
	// below fields are guarded by c.mtx
	// cbs is the list of change callbacks
	cbs []unixfs.FSCursorChangeCb
}

// newRemoteFSCursor constructs a new remote FSCursor.
func newRemoteFSCursor(c *FSCursorClient, cursorHandleID uint64) *remoteFSCursor {
	return &remoteFSCursor{c: c, cursorHandleID: cursorHandleID}
}

// CheckReleased checks if the fs cursor is currently released.
func (c *remoteFSCursor) CheckReleased() bool {
	return c.released.Load()
}

// GetProxyCursor returns a FSCursor to replace this one, if necessary.
// This is used to resolve a symbolic link, mount, etc.
// Return nil, nil if no redirection necessary (in most cases).
// This will be called before any of the other calls.
// Releasing a child cursor does not release the parent, and vise-versa.
// Return nil, ErrReleased if this FSCursor was released.
func (c *remoteFSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	resp, err := c.c.client.GetProxyCursor(ctx, &unixfs_rpc.GetProxyCursorRequest{
		CursorHandleId: c.cursorHandleID,
		ClientHandleId: c.c.clientHandleID,
	})
	if err == nil {
		err = resp.GetUnixfsError().ToGoError()
	}
	if err != nil {
		if err == unixfs_errors.ErrReleased {
			c.Release()
		}
		return nil, err
	}

	cursorHandleID := resp.GetCursorHandleId()
	if cursorHandleID == 0 {
		return nil, nil
	}

	c.c.mtx.Lock()
	defer c.c.mtx.Unlock()

	if c.c.released.Load() {
		return nil, unixfs_errors.ErrReleased
	}

	cursor := c.c.ingestCursorLocked(cursorHandleID)
	if cursor == nil {
		return nil, unixfs_errors.ErrReleased
	}

	return cursor, nil
}

// AddChangeCb adds a change callback to detect when the cursor has changed.
// This will be called only if GetProxyCursor returns nil, nil.
//
// cb must not block, and should be called when cursor changes / is released
// cb will be called immediately (same call tree) if already released.
func (c *remoteFSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	c.c.mtx.Lock()
	if !c.released.Load() && !c.c.released.Load() {
		c.cbs = append(c.cbs, cb)
	} else {
		defer cb(&unixfs.FSCursorChange{Released: true})
	}
	c.c.mtx.Unlock()
}

// GetCursorOps returns the FSCursorOps for the FSCursor.
// Called after AddChangeCb and only if GetProxyCursor returns nil, nil.
// Returning nil, nil will be corrected to nil, ErrNotExist.
// Return nil, ErrReleased to indicate this FSCursor was released.
func (c *remoteFSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	resp, err := c.c.client.GetCursorOps(ctx, &unixfs_rpc.GetCursorOpsRequest{
		CursorHandleId: c.cursorHandleID,
	})
	if err == nil {
		err = resp.GetUnixfsError().ToGoError()
	}
	if err != nil {
		if err == unixfs_errors.ErrReleased {
			c.Release()
		}
		return nil, err
	}

	opsHandleID := resp.GetOpsHandleId()
	if opsHandleID == 0 {
		return nil, unixfs_rpc.ErrHandleIDEmpty
	}

	c.c.mtx.Lock()
	defer c.c.mtx.Unlock()

	if c.c.released.Load() {
		return nil, unixfs_errors.ErrReleased
	}

	nodeType := resp.GetNodeType()
	name := resp.GetName()

	retOps, retOpsOk := c.c.ops[opsHandleID]
	if !retOpsOk || retOps == nil || retOps.released.Load() || retOps.name != name || retOps.nodeType != nodeType {
		retOps = newRemoteFSCursorOps(c, opsHandleID, nodeType, resp.GetName())
		existingOpsID, existingOpsIDOk := c.c.cursorOps[c.cursorHandleID]
		if existingOpsIDOk {
			existingOps, existingOpsOk := c.c.ops[existingOpsID]
			if existingOpsOk {
				existingOps.released.Store(true)
				delete(c.c.ops, existingOpsID)
			}
		}
		c.c.ops[opsHandleID] = retOps
		c.c.cursorOps[c.cursorHandleID] = opsHandleID
	}

	// make sure we don't accidentally return an untyped nil
	if retOps == nil {
		return nil, nil
	}

	return retOps, nil
}

// Release releases the filesystem cursor.
func (c *remoteFSCursor) Release() {
	if !c.released.Swap(true) {
		_, _ = c.c.client.ReleaseFSCursor(c.c.ctx, &unixfs_rpc.ReleaseFSCursorRequest{
			CursorHandleId: c.cursorHandleID,
			ClientHandleId: c.c.clientHandleID,
		})
	}
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*remoteFSCursor)(nil))
