package unixfs_rpc_client

import (
	"context"
	"errors"
	"sync"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
)

// FSCursorClient is the cursor returned by GetProxyCursor on FSCursor.
// It is recommended to use FSCursor instead as it will automatically re-connect.
//
// this cursor manages the event watching loop on FSCursor.
// the cursor will be released if the FSCursorClient RPC is canceled.
type FSCursorClient struct {
	// remoteFSCursor implements FSCursor with remote rootCursorHandleID.
	*remoteFSCursor
	// ctx is the context to use for global operations (release handle)
	ctx context.Context
	// cancel cancels the context for the cursor client
	cancel context.CancelFunc
	// client is the client object for requests
	client unixfs_rpc.SRPCFSCursorServiceClient
	// clientHandleID is the client handle id to use for requests.
	clientHandleID uint64
	// mtx guards below fields
	mtx sync.Mutex
	// cursors contains the mapping from remote cursor id to cursor
	cursors map[uint64]*remoteFSCursor
	// ops contains the mapping from remote ops id to ops
	ops map[uint64]*remoteFSCursorOps
	// cursorOps contains the mapping from remote cursor id to remote ops id
	cursorOps map[uint64]uint64
}

// BuildFSCursorClient constructs & initializes the FSCursorClient, starting the
// management goroutine and initializing a client id with the remote service.
//
// does not return until the init message is received from the remote.
// the context is used for the persistent goroutine.
func BuildFSCursorClient(rctx context.Context, client unixfs_rpc.SRPCFSCursorServiceClient) (*FSCursorClient, error) {
	ctx, ctxCancel := context.WithCancel(rctx)
	strm, err := client.FSCursorClient(ctx, &unixfs_rpc.FSCursorClientRequest{})
	if err != nil {
		ctxCancel()
		return nil, err
	}

	resp, err := strm.Recv()
	if err != nil {
		ctxCancel()
		_ = strm.Close()
		return nil, err
	}

	// handle error response
	if err := resp.GetUnixfsError().ToGoError(); err != nil {
		ctxCancel()
		_ = strm.Close()
		return nil, err
	}

	// handle successful init
	initMsg := resp.GetInit()
	if initMsg == nil {
		ctxCancel()
		_ = strm.Close()
		return nil, errors.New("unexpected non-init msg as first response to FSCursorClient")
	}

	clientHandleID, rootCursorHandleID := initMsg.GetClientHandleId(), initMsg.GetCursorHandleId()
	if clientHandleID == 0 {
		ctxCancel()
		_ = strm.Close()
		return nil, errors.New("unexpected empty client handle id in fs cursor client init")
	}
	if rootCursorHandleID == 0 {
		ctxCancel()
		_ = strm.Close()
		return nil, errors.New("unexpected empty root cursor handle id in fs cursor client init")
	}

	fsc := &FSCursorClient{
		ctx:            ctx,
		cancel:         ctxCancel,
		client:         client,
		clientHandleID: clientHandleID,
		cursors:        make(map[uint64]*remoteFSCursor, 1),
		ops:            make(map[uint64]*remoteFSCursorOps),
		cursorOps:      make(map[uint64]uint64),
	}
	fsc.remoteFSCursor = newRemoteFSCursor(fsc, rootCursorHandleID)
	fsc.cursors[rootCursorHandleID] = fsc.remoteFSCursor
	go fsc.execute(ctx, strm)

	return fsc, nil
}

// execute is the goroutine managing the FSCursorClient.
func (c *FSCursorClient) execute(ctx context.Context, strm unixfs_rpc.SRPCFSCursorService_FSCursorClientClient) {
	defer func() {
		c.Release()
		c.mtx.Lock()
		for _, ops := range c.ops {
			ops.released.Store(true)
		}
		c.ops = make(map[uint64]*remoteFSCursorOps, 0)
		for _, remoteCursor := range c.cursors {
			remoteCursor.released.Store(true)
		}
		c.cursors = make(map[uint64]*remoteFSCursor, 0)
		c.cursorOps = make(map[uint64]uint64, 0)
		c.mtx.Unlock()
		_ = strm.Close()
	}()

	msg := &unixfs_rpc.FSCursorClientResponse{}
	for {
		msg.Reset()
		if err := strm.RecvTo(msg); err != nil {
			return
		}

		switch resp := msg.GetBody().(type) {
		case *unixfs_rpc.FSCursorClientResponse_CursorChange:
			if ch := resp.CursorChange; ch != nil {
				c.handleCursorChange(ctx, ch)
			}
		}
	}
}

// handleCursorChange handles an incoming cursor change message.
func (c *FSCursorClient) handleCursorChange(ctx context.Context, ch *unixfs_rpc.FSCursorChange) {
	cursorHandleID := ch.GetCursorHandleId()
	if cursorHandleID == 0 {
		return
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

	cursor, ok := c.cursors[cursorHandleID]
	if !ok {
		return
	}

	// if released, remove from the cursors set.
	if ch.Released {
		cursor.released.Store(true)
		delete(c.cursors, cursorHandleID)

		// release associated ops object, if applicable.
		opsID, opsOk := c.cursorOps[cursorHandleID]
		if opsOk {
			delete(c.cursorOps, cursorHandleID)
			ops, opsOk := c.ops[opsID]
			if opsOk {
				ops.released.Store(true)
				delete(c.ops, opsID)
			}
		}
	}

	// fire the event to any listeners
	if len(cursor.cbs) != 0 {
		changeObj := ch.ToFSCursorChange()
		for _, cb := range cursor.cbs {
			if cb != nil {
				cb(changeObj)
			}
		}
	}
}

// ingestCursorLocked ingests a FSCursor while mtx is locked.
func (c *FSCursorClient) ingestCursorLocked(cursorHandleID uint64) *remoteFSCursor {
	retCursor, retCursorOk := c.c.cursors[cursorHandleID]
	if !retCursorOk || retCursor.released.Load() {
		retCursor = newRemoteFSCursor(c.c, cursorHandleID)
		c.c.cursors[cursorHandleID] = retCursor
	}
	return retCursor
}

// Release releases the filesystem cursor client.
// All sub-cursors will be automatically canceled as well.
func (c *FSCursorClient) Release() {
	c.released.Store(true)
	c.cancel()
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*FSCursor)(nil))
