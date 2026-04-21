package web_pkg_rpc_server

import (
	"context"

	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_rpc "github.com/s4wave/spacewave/bldr/web/pkg/rpc"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_server "github.com/s4wave/spacewave/db/unixfs/rpc/server"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// WebPkgServer implements the WebPkg service server.
// Serves AccessWebPkg and FSCursorService on the same service ID.
type WebPkgServer struct {
	le    *logrus.Entry
	pkg   web_pkg.WebPkg
	fsMux srpc.Mux
}

// NewWebPkgServer constructs a new web pkg server.
func NewWebPkgServer(le *logrus.Entry, pkg web_pkg.WebPkg) *WebPkgServer {
	s := &WebPkgServer{le: le, pkg: pkg}
	mux := srpc.NewMux()
	rootFSCursor := unixfs.NewFSCursorGetter(func(ctx context.Context) (unixfs.FSCursor, error) {
		fsHandle, err := pkg.GetWebPkgFsHandle(ctx)
		if err != nil {
			return nil, err
		}

		return unixfs.NewFSHandleCursor(fsHandle, true, nil), nil
	})
	_ = unixfs_rpc.SRPCRegisterFSCursorService(
		mux,
		unixfs_rpc_server.NewFSCursorService(rootFSCursor),
	)
	s.fsMux = mux
	return s
}

// GetWebPkgInfo returns the information about the web pkg.
func (s *WebPkgServer) GetWebPkgInfo(
	ctx context.Context,
	req *web_pkg_rpc.GetInfoRequest,
) (*web_pkg_rpc.GetInfoResponse, error) {
	info, err := s.pkg.GetInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &web_pkg_rpc.GetInfoResponse{Info: info}, nil
}

// WebPkgFsRpc performs an RPC against the web pkg filesystem FSCursorService.
func (s *WebPkgServer) WebPkgFsRpc(strm web_pkg_rpc.SRPCAccessWebPkg_WebPkgFsRpcStream) error {
	return rpcstream.HandleRpcStream(strm, s.GetWebPkgFsMux)
}

// GetWebPkgFsMux returns the mux for the web pkg fs service.
func (s *WebPkgServer) GetWebPkgFsMux(ctx context.Context, _ string, _ func()) (srpc.Invoker, func(), error) {
	return s.fsMux, nil, nil
}

// _ is a type assertion
var _ web_pkg_rpc.SRPCAccessWebPkgServer = (*WebPkgServer)(nil)
