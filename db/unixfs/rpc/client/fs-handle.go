package unixfs_rpc_client

import (
	"context"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
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

// NewFSHandleBuilder constructs a new FSHandleBuilder with a FSCursorServiceClient.
func NewFSHandleBuilder(fsCursorSvcClient unixfs_rpc.SRPCFSCursorServiceClient) unixfs.FSHandleBuilder {
	return func(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
		fs, err := BuildFSHandle(ctx, fsCursorSvcClient)
		if err != nil {
			return nil, nil, err
		}
		fs.AddReleaseCallback(released)
		return fs, fs.Release, nil
	}
}
