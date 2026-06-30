package s4wave_sshhost

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
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
	ctx := strm.Context()

	objState, found, err := r.ws.GetObject(ctx, r.objKey)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	var lastSent *SshHost
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		_, rev, err := objState.GetRootRef(ctx)
		if err != nil {
			return err
		}

		state, err := readSshHostObject(ctx, objState)
		if err != nil {
			return err
		}
		if state == nil {
			state = &SshHost{}
		}

		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.state = state.CloneVT()
			broadcast()
		})

		if lastSent == nil || !state.EqualVT(lastSent) {
			if serr := strm.Send(&WatchSshHostStateResponse{State: state.CloneVT()}); serr != nil {
				return serr
			}
			lastSent = state
		}

		_, err = objState.WaitRev(ctx, rev+1, false)
		if err != nil {
			return err
		}
	}
}

func readSshHostObject(ctx context.Context, objState world.ObjectState) (*SshHost, error) {
	var state *SshHost
	_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = UnmarshalSshHost(ctx, bcs)
		return uerr
	})
	return state, err
}

var _ SRPCSshHostResourceServiceServer = (*SshHostResource)(nil)
