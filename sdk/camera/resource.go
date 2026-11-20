package s4wave_camera

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_frametrack "github.com/s4wave/spacewave/sdk/frametrack"
)

// CameraResource watches a Camera object and plays its track.
type CameraResource struct {
	// track follows the Camera's Device and capability.
	track *s4wave_frametrack.Client
	// camera is the immutable Camera record held by the resource.
	camera *Camera
	// mux dispatches Camera resource methods.
	mux srpc.Mux
}

// NewCameraResource creates a CameraResource for a viewer's Session.
func NewCameraResource(b bus.Bus, ws world.WorldState, sessionPeer peer.ID, camera *Camera) *CameraResource {
	// Resolve the track through the Device object for each connection.
	track := s4wave_frametrack.NewClient(
		b, ws, sessionPeer, camera.GetDeviceObjectKey(), camera.GetCapabilityId(), s4wave_device.DeviceCapabilityKindCamera,
	)

	// Bind the watch and track to this Camera resource.
	r := &CameraResource{track: track, camera: camera}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterCameraResourceService(mux, r)
	})
	return r
}

// GetMux returns the SRPC mux for this resource.
func (r *CameraResource) GetMux() srpc.Mux {
	return r.mux
}

// WatchCamera reports the Camera and the availability of its Device.
func (r *CameraResource) WatchCamera(_ *WatchCameraRequest, strm SRPCCameraResourceService_WatchCameraStream) error {
	return r.track.WatchDevice(strm.Context(), func(device *s4wave_device.DeviceAvailability) error {
		return strm.Send(&WatchCameraResponse{Camera: r.camera.CloneVT(), Device: device})
	})
}

// WatchFrames plays the Camera's view-only track on its Device.
func (r *CameraResource) WatchFrames(_ *WatchFramesRequest, strm SRPCCameraResourceService_WatchFramesStream) error {
	return r.track.Play(strm.Context(), nil, strm.Send)
}

// _ is a type assertion.
var _ SRPCCameraResourceServiceServer = (*CameraResource)(nil)
