package unixfs_rpc_client

import (
	"context"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_rpc "github.com/aperturerobotics/hydra/unixfs/rpc"
)

// BuildFSHandle constructs a root fs handle from a FSCursor service.
//
// rctx is used as the root context for the rpc client
// rctx must remain not-canceled during the duration of the lifetime of the FSHandle.
func BuildFSHandle(rctx context.Context, fsCursorSvcClient unixfs_rpc.SRPCFSCursorServiceClient) (*unixfs.FSHandle, error) {
	fsCursor := NewFSCursor(rctx, fsCursorSvcClient)
	fs, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		fsCursor.Release()
		return nil, err
	}
	return fs, nil
}
