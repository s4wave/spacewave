package unixfs_rpc_client

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_rpc "github.com/aperturerobotics/hydra/unixfs/rpc"
)

// FSCursor implements a FSCursor on top of the FSCursorService.
//
// The first cursor returned from GetProxyCursor manages the event stream.
type FSCursor struct {
	// released indicates the cursor has been released
	released atomic.Bool
	// ctx is the context for the client
	ctx context.Context
	// client is the fs cursor service client
	client unixfs_rpc.SRPCFSCursorServiceClient
}

// NewFSCursor constructs a new FSCursor from a RPC service client.
//
// ctx is the root cursor to use for the cursor service long-lived client request.
func NewFSCursor(ctx context.Context, client unixfs_rpc.SRPCFSCursorServiceClient) *FSCursor {
	return &FSCursor{
		ctx:    ctx,
		client: client,
	}
}

// CheckReleased checks if the fs cursor is currently released.
func (c *FSCursor) CheckReleased() bool {
	return c.released.Load()
}

// GetProxyCursor returns a FSCursor to replace this one, if necessary.
// This is used to resolve a symbolic link, mount, etc.
// Return nil, nil if no redirection necessary (in most cases).
// This will be called before any of the other calls.
// Releasing a child cursor does not release the parent, and vise-versa.
// Return nil, ErrReleased if this FSCursor was released.
func (c *FSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	// initialize the client session with the remote
	rootFSCursor, err := BuildFSCursorClient(c.ctx, c.client)
	if err != nil {
		return nil, err
	}

	return rootFSCursor, nil
}

// Release releases the filesystem cursor.
func (c *FSCursor) Release() {
	c.released.Store(true)
}

// AddChangeCb is not applicable to the root client FSCursor as GetProxyCursor
// always returns a sub-cursor.
func (c *FSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {}

// GetCursorOps is not applicable to the root client FSCursor as GetProxyCursor
// always returns a sub-cursor.
func (c *FSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) { return nil, nil }

// _ is a type assertion
var _ unixfs.FSCursor = ((*FSCursor)(nil))
