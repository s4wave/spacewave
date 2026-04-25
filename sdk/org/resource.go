package s4wave_org

import (
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
)

// OrgResource implements the OrgResourceService SRPC interface.
type OrgResource struct {
	ws     world.WorldState
	objKey string
	state  *OrgState
	bcast  broadcast.Broadcast
	mux    srpc.Mux
}

// NewOrgResource creates a new OrgResource.
func NewOrgResource(ws world.WorldState, objKey string, state *OrgState) *OrgResource {
	if state == nil {
		state = &OrgState{}
	}
	r := &OrgResource{
		ws:     ws,
		objKey: objKey,
		state:  state,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterOrgResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *OrgResource) GetMux() srpc.Mux {
	return r.mux
}

// WatchOrgState streams organization state changes.
func (r *OrgResource) WatchOrgState(_ *WatchOrgStateRequest, strm SRPCOrgResourceService_WatchOrgStateStream) error {
	return broadcast.WatchBroadcast(
		strm.Context(), &r.bcast,
		func() *OrgState { return r.state.CloneVT() },
		func(s *OrgState) error { return strm.Send(&WatchOrgStateResponse{State: s}) },
	)
}

// _ is a type assertion
var _ SRPCOrgResourceServiceServer = (*OrgResource)(nil)
