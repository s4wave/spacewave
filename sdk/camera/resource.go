package s4wave_camera

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/frametrack"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// errTrackEnded is the cause recorded when the Device closes the control
// stream, which ends the frame track without an error.
var errTrackEnded = errors.New("the Device ended the frame track")

// errSessionStopped is the cause recorded when the viewer's Session transport
// stops under a playing frame track.
var errSessionStopped = errors.New("the viewer's session transport stopped")

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

// WatchFrames opens the Camera's frame track on its Device over the viewer's
// Session, sends Opened once the Device holds the subscription, and then sends
// the newest picture each time the caller has taken the last one. It returns
// nil when the Device closes the track, and an error when the caller, the
// Session transport or the link ends it first.
func (r *CameraResource) WatchFrames(_ *WatchFramesRequest, strm SRPCCameraResourceService_WatchFramesStream) error {
	// End the track's streams with the call or the Session transport.
	ctx, cancel := context.WithCancelCause(strm.Context())
	defer cancel(nil)

	// Resolve the Device's peer and the protocol its camera capability serves.
	devicePeer, protocolID, err := r.resolveCameraLink(ctx)
	if err != nil {
		return err
	}

	// Borrow the viewer's Session transport, where the Device's link resolves.
	st, releaseSession, err := transport.ResolveSessionTransport(ctx, r.b, r.sessionPeer, func() {
		cancel(errSessionStopped)
	})
	if err != nil {
		return err
	}
	defer releaseSession()
	if st == nil {
		return errors.New("no session transport for the viewer's session")
	}
	sessionBus := st.GetChildBus()
	if sessionBus == nil {
		return errSessionStopped
	}

	// Open the control stream, which holds the link the groups arrive on.
	ms, releaseLink, err := link.OpenStreamWithPeerEx(ctx, sessionBus, protocolID, st.GetPeerID(), devicePeer, 0, stream.OpenOpts{})
	if err != nil {
		return err
	}
	defer releaseLink()
	control := ms.GetStream()

	// Register the subscription before the Device can send its first group.
	var slot frameSlot
	subscription, releaseSubscription, err := st.GetFrameSubscriptions().Register(ctx, ms, slot.readGroup)
	if err != nil {
		_ = control.Close()
		return err
	}
	defer releaseSubscription()

	// The Device sends nothing on the control stream; its end ends the track.
	// Closing the stream on return releases this reader, which the call joins.
	controlDone := make(chan struct{})
	go func() {
		defer close(controlDone)
		var extra [1]byte
		_, _ = control.Read(extra[:])
		cancel(errTrackEnded)
	}()
	defer func() {
		_ = control.Close()
		<-controlDone
	}()

	// Send the subscription and report the open track.
	if err := frametrack.WriteSubscription(control, subscription); err != nil {
		return err
	}
	if err := strm.Send(&WatchFramesResponse{State: &WatchFramesResponse_Opened{Opened: &WatchFramesOpened{}}}); err != nil {
		return err
	}

	// Send the newest picture each time the caller takes one.
	for {
		frame, err := slot.take(ctx)
		if err != nil {
			if errors.Is(err, errTrackEnded) {
				return nil
			}
			return err
		}
		if err := strm.Send(&WatchFramesResponse{State: &WatchFramesResponse_Frame{Frame: frame}}); err != nil {
			return err
		}
	}
}

// resolveCameraLink reads the Camera's Device and returns the Device's peer and
// the control protocol its camera capability publishes.
func (r *CameraResource) resolveCameraLink(ctx context.Context) (peer.ID, protocol.ID, error) {
	// Read the Device object the Camera names.
	objState, found, err := r.ws.GetObject(ctx, r.camera.GetDeviceObjectKey())
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return "", "", err
	}
	if !found {
		return "", "", world.ErrObjectNotFound
	}
	device, err := s4wave_world.ReadWorldBlock[s4wave_device.Device](ctx, objState)
	if err != nil {
		return "", "", err
	}

	// Decode the Device's peer, which the Device object names.
	devicePeer, err := peer.IDB58Decode(device.GetPeerId())
	if err != nil {
		return "", "", errors.Wrap(err, "camera device peer id")
	}

	// Find the capability and require it to serve on a valid protocol.
	capabilities := device.GetCapabilities()
	i := slices.IndexFunc(capabilities, func(c *s4wave_device.DeviceCapability) bool {
		return c.GetId() == r.camera.GetCapabilityId()
	})
	if i < 0 {
		return "", "", errors.New("the Device does not publish this camera")
	}
	capability := capabilities[i]
	if !s4wave_device.DeviceCapabilityIsSelectable(capability) {
		return "", "", errors.Errorf("the Device's camera is not running: %s", capability.GetDetail())
	}
	protocolID := protocol.ID(capability.GetLink().GetProtocolId())
	if err := protocolID.Validate(); err != nil {
		return "", "", errors.Wrap(err, "camera control protocol")
	}
	return devicePeer, protocolID, nil
}

// _ is a type assertion
var _ SRPCCameraResourceServiceServer = (*CameraResource)(nil)
