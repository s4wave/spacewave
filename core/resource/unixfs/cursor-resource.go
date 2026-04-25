package resource_unixfs

import (
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_server "github.com/s4wave/spacewave/db/unixfs/rpc/server"
)

// FSCursorResource wraps a hydra FSCursorService and serves it over a resource mux.
// Each instance maps 1:1 to an FSCursor on the server side, exposing all 22
// FSCursorService RPCs (cursor management, ops, read/write, directory, etc.).
type FSCursorResource struct {
	svc *unixfs_rpc_server.FSCursorService
	mux srpc.Mux
}

// NewFSCursorResource creates a new FSCursorResource from an FSCursor.
func NewFSCursorResource(cursor unixfs.FSCursor) *FSCursorResource {
	svc := unixfs_rpc_server.NewFSCursorService(cursor)
	r := &FSCursorResource{svc: svc}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return unixfs_rpc.SRPCRegisterFSCursorService(mux, svc)
	})
	return r
}

// NewFSCursorResourceWithHandle creates a new FSCursorResource from an FSHandle.
// The FSHandle is wrapped in an FSCursorGetter so the cursor is reconstructed
// automatically when the service is accessed.
func NewFSCursorResourceWithHandle(handle *unixfs.FSHandle) *FSCursorResource {
	svc := unixfs_rpc_server.NewFSCursorServiceWithHandle(handle)
	r := &FSCursorResource{svc: svc}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return unixfs_rpc.SRPCRegisterFSCursorService(mux, svc)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *FSCursorResource) GetMux() srpc.Mux {
	return r.mux
}

// GetService returns the underlying FSCursorService.
func (r *FSCursorResource) GetService() *unixfs_rpc_server.FSCursorService {
	return r.svc
}

// Release releases all contents of the service including the root cursor.
func (r *FSCursorResource) Release() {
	r.svc.Release(true)
}
