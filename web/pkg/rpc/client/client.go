package web_pkg_rpc_client

import (
	"context"

	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	web_pkg_rpc "github.com/aperturerobotics/bldr/web/pkg/rpc"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_rpc "github.com/aperturerobotics/hydra/unixfs/rpc"
	unixfs_rpc_client "github.com/aperturerobotics/hydra/unixfs/rpc/client"
	"github.com/aperturerobotics/starpc/rpcstream"
)

// RemoteWebPkg implements web/pkg.WebPkg backed by web/pkg/rpc.
type RemoteWebPkg struct {
	id     string
	client web_pkg_rpc.SRPCAccessWebPkgClient
	fsh    *unixfs.FSHandle
}

// NewRemoteWebPkg constructs a new remote web pkg client.
//
// The context is used for the long-lived fs handle client call.
func NewRemoteWebPkg(
	ctx context.Context,
	id string,
	client web_pkg_rpc.SRPCAccessWebPkgClient,
) (*RemoteWebPkg, error) {
	fsRpcClient := rpcstream.NewRpcStreamClient(client.WebPkgFsRpc, "", false)
	// verboseClient := srpc.NewVClient(fsRpcClient, le)
	// fsRpcClient = verboseClient
	fsRpcSrvClient := unixfs_rpc.NewSRPCFSCursorServiceClient(fsRpcClient)
	fsc := unixfs_rpc_client.NewFSCursor(ctx, fsRpcSrvClient)
	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		return nil, err
	}

	return &RemoteWebPkg{id: id, client: client, fsh: fsh}, nil
}

// GetId returns the web package identifier.
func (r *RemoteWebPkg) GetId() string {
	return r.id
}

// GetInfo returns the WebPkgInfo for the WebPkg.
func (r *RemoteWebPkg) GetInfo(ctx context.Context) (*web_pkg.WebPkgInfo, error) {
	resp, err := r.client.GetWebPkgInfo(ctx, &web_pkg_rpc.GetInfoRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetInfo(), nil
}

// GetWebPkgFsHandle returns a fs cursor which can be used to access the WebPkg fs.
// Use unixfs.NewFSCursorGetter if async lookup logic is required.
func (r *RemoteWebPkg) GetWebPkgFsHandle(ctx context.Context) (*unixfs.FSHandle, error) {
	return r.fsh.Clone(ctx)
}

// Release releases the internal fs cursor.
func (r *RemoteWebPkg) Release() {
	r.fsh.Release()
}

// _ is a type assertion
var _ web_pkg.WebPkg = ((*RemoteWebPkg)(nil))
