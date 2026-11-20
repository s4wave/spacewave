package s4wave_display

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_frametrack "github.com/s4wave/spacewave/sdk/frametrack"
)

// DisplayResource watches a Display and connects its frames and input.
type DisplayResource struct {
	// track follows the Display's Device and capability.
	track *s4wave_frametrack.Client
	// display is the immutable Display record held by the resource.
	display *Display
	// mux dispatches Display resource methods.
	mux srpc.Mux
}

// NewDisplayResource creates a DisplayResource for a viewer's Session.
func NewDisplayResource(b bus.Bus, ws world.WorldState, sessionPeer peer.ID, display *Display) *DisplayResource {
	// Resolve the track through the Device object for each connection.
	track := s4wave_frametrack.NewClient(
		b, ws, sessionPeer, display.GetDeviceObjectKey(), display.GetCapabilityId(), s4wave_device.DeviceCapabilityKindDisplay,
	)

	// Bind both methods to this Display resource.
	r := &DisplayResource{track: track, display: display}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterDisplayResourceService(mux, r)
	})
	return r
}

// GetMux returns the SRPC mux for this resource.
func (r *DisplayResource) GetMux() srpc.Mux {
	return r.mux
}

// WatchDisplay reports the Display and the availability of its Device.
func (r *DisplayResource) WatchDisplay(_ *WatchDisplayRequest, strm SRPCDisplayResourceService_WatchDisplayStream) error {
	return r.track.WatchDevice(strm.Context(), func(device *s4wave_device.DeviceAvailability) error {
		return strm.Send(&WatchDisplayResponse{Display: r.display.CloneVT(), Device: device})
	})
}

// ConnectDisplay plays the Display track and forwards this viewer's input.
func (r *DisplayResource) ConnectDisplay(strm SRPCDisplayResourceService_ConnectDisplayStream) error {
	return r.track.Play(strm.Context(), strm.Recv, strm.Send)
}

// _ is a type assertion.
var _ SRPCDisplayResourceServiceServer = (*DisplayResource)(nil)
