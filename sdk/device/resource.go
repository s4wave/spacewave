package s4wave_device

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// DeviceResource implements the DeviceResourceService SRPC interface.
type DeviceResource struct {
	ws     world.WorldState
	objKey string
	state  *Device
	bcast  broadcast.Broadcast
	mux    srpc.Mux
}

// NewDeviceResource creates a new DeviceResource.
func NewDeviceResource(ws world.WorldState, objKey string, state *Device) *DeviceResource {
	if state == nil {
		state = &Device{}
	}
	r := &DeviceResource{
		ws:     ws,
		objKey: objKey,
		state:  state,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterDeviceResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *DeviceResource) GetMux() srpc.Mux {
	return r.mux
}

// WatchDeviceState streams Device state changes from world object revisions.
func (r *DeviceResource) WatchDeviceState(_ *WatchDeviceStateRequest, strm SRPCDeviceResourceService_WatchDeviceStateStream) error {
	ctx := strm.Context()

	objState, found, err := r.ws.GetObject(ctx, r.objKey)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	var lastSent *Device
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		_, rev, err := objState.GetRootRef(ctx)
		if err != nil {
			return err
		}

		state, err := readDeviceObject(ctx, objState)
		if err != nil {
			return err
		}
		if state == nil {
			state = &Device{}
		}

		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.state = state.CloneVT()
			broadcast()
		})

		if lastSent == nil || !state.EqualVT(lastSent) {
			if serr := strm.Send(&WatchDeviceStateResponse{State: state.CloneVT()}); serr != nil {
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

// ReportDeviceStatus rejects browser-visible status writes.
func (r *DeviceResource) ReportDeviceStatus(ctx context.Context, req *ReportDeviceStatusRequest) (*ReportDeviceStatusResponse, error) {
	return nil, errors.New("device status reports are daemon-owned and unavailable through typed object resources")
}

func readDeviceObject(ctx context.Context, objState world.ObjectState) (*Device, error) {
	var state *Device
	_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = UnmarshalDevice(ctx, bcs)
		return uerr
	})
	return state, err
}

// _ is a type assertion
var _ SRPCDeviceResourceServiceServer = (*DeviceResource)(nil)
