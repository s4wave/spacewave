package s4wave_camera

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// CameraResource implements the CameraResourceService SRPC interface.
type CameraResource struct {
	// b resolves the viewer's Session transport.
	b bus.Bus
	// ws is the granted World state.
	ws world.WorldState
	// sessionPeer is the viewer's Session peer, which the Device's
	// reachability is measured from.
	sessionPeer peer.ID
	// camera is the Camera record. No op changes a Camera after it is
	// created, so the record holds for the resource's lifetime.
	camera *Camera
	// mux dispatches Camera resource methods.
	mux srpc.Mux
}

// NewCameraResource creates a CameraResource for the camera record, seen from
// the viewer's Session sessionPeer.
func NewCameraResource(b bus.Bus, ws world.WorldState, sessionPeer peer.ID, camera *Camera) *CameraResource {
	r := &CameraResource{b: b, ws: ws, sessionPeer: sessionPeer, camera: camera}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterCameraResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *CameraResource) GetMux() srpc.Mux {
	return r.mux
}

// WatchCamera sends the Camera with its Device's availability, then again on
// each change of the Device object or of its link to the viewer's Session.
// The Device's peer comes from the Device object, so a re-enrolled Device is
// followed without changing the Camera.
func (r *CameraResource) WatchCamera(_ *WatchCameraRequest, strm SRPCCameraResourceService_WatchCameraStream) error {
	return s4wave_device.WatchDeviceAvailability(
		strm.Context(),
		r.b,
		r.sessionPeer,
		r.ws,
		r.camera.GetDeviceObjectKey(),
		func(device *s4wave_device.DeviceAvailability) error {
			return strm.Send(&WatchCameraResponse{Camera: r.camera.CloneVT(), Device: device})
		},
	)
}

// _ is a type assertion
var _ SRPCCameraResourceServiceServer = (*CameraResource)(nil)
