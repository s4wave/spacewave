package s4wave_sshhost

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// SshHostResource implements the SshHostResourceService SRPC interface.
type SshHostResource struct {
	ws     world.WorldState
	objKey string
	state  *SshHost
	bcast  broadcast.Broadcast
	mux    srpc.Mux
}

// NewSshHostResource creates a new SshHostResource.
func NewSshHostResource(ws world.WorldState, objKey string, state *SshHost) *SshHostResource {
	if state == nil {
		state = &SshHost{}
	}
	r := &SshHostResource{
		ws:     ws,
		objKey: objKey,
		state:  state,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterSshHostResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *SshHostResource) GetMux() srpc.Mux {
	return r.mux
}

// WatchSshHostState streams SSH Host state changes from world object revisions.
func (r *SshHostResource) WatchSshHostState(_ *WatchSshHostStateRequest, strm SRPCSshHostResourceService_WatchSshHostStateStream) error {
	return s4wave_world.WatchWorldObject(strm.Context(), r.ws, r.objKey, readSshHostObject,
		func(state *SshHost, changed bool) error {
			r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				r.state = state.CloneVT()
				broadcast()
			})
			if changed {
				return strm.Send(&WatchSshHostStateResponse{State: state.CloneVT()})
			}
			return nil
		})
}

func readSshHostObject(ctx context.Context, objState world.ObjectState) (*SshHost, error) {
	state, err := s4wave_world.ReadWorldBlock[*SshHost](ctx, objState, NewSshHostBlock)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &SshHost{}
	}
	return state, nil
}

var _ SRPCSshHostResourceServiceServer = (*SshHostResource)(nil)
