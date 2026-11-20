// Package s4wave_frametrack connects viewers to tracks served by Device capabilities.
package s4wave_frametrack

import (
	"context"
	"io"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/frametrack"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

var (
	// errTrackEnded distinguishes a Device closing its track from a failed call.
	errTrackEnded = errors.New("the Device ended the frame track")
	// errSessionStopped reports that the viewer's Session transport stopped.
	errSessionStopped = errors.New("the viewer's session transport stopped")
)

// Client resolves a frame track from its Device object and capability.
type Client struct {
	// b resolves the transport running the viewer's Session.
	b bus.Bus
	// ws is the viewer's granted World state.
	ws world.WorldState
	// sessionPeer is the local Session identity of the viewer.
	sessionPeer peer.ID
	// deviceKey names the Device object serving the track.
	deviceKey string
	// capabilityID selects the capability on the Device.
	capabilityID string
	// kind is the kind of capability the viewer can open.
	kind string
}

// NewClient uses the viewer's Session and World to follow the named Device.
func NewClient(b bus.Bus, ws world.WorldState, sessionPeer peer.ID, deviceKey, capabilityID, kind string) *Client {
	return &Client{b: b, ws: ws, sessionPeer: sessionPeer, deviceKey: deviceKey, capabilityID: capabilityID, kind: kind}
}

// WatchDevice reports the Device's reachability from the viewer's Session.
func (c *Client) WatchDevice(ctx context.Context, send func(*s4wave_device.DeviceAvailability) error) error {
	return s4wave_device.WatchDeviceAvailability(ctx, c.b, c.sessionPeer, c.ws, c.deviceKey, send)
}

// Play opens the track and delivers its pictures through send until either end
// closes. If recv is supplied, it supplies the input of this same resource
// call. The resource stream must release a blocked recv when the call ends.
func (c *Client) Play(
	ctx context.Context,
	recv func() (*frametrack.Input, error),
	send func(*frametrack.FrameEvent) error,
) error {
	// End the control stream and relay with the caller or the Session transport.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	// Read the Device and find the protocol its capability serves today.
	devicePeer, protocolID, err := c.resolveLink(ctx)
	if err != nil {
		return err
	}
	st, releaseSession, err := transport.ResolveSessionTransport(ctx, c.b, c.sessionPeer, func() {
		cancel(errSessionStopped)
	})
	if err != nil {
		return err
	}
	defer releaseSession()
	if st == nil {
		return errSessionStopped
	}
	sessionBus := st.GetChildBus()
	if sessionBus == nil {
		return errSessionStopped
	}

	// The control stream holds the link on which the Device sends its groups.
	ms, releaseLink, err := link.OpenStreamWithPeerEx(ctx, sessionBus, protocolID, st.GetPeerID(), devicePeer, 0, stream.OpenOpts{})
	if err != nil {
		return err
	}
	defer releaseLink()
	control := ms.GetStream()
	stopClose := context.AfterFunc(ctx, func() {
		// Closing the control stream releases any blocked input write or read.
		_ = control.Close()
	})
	defer func() {
		if stopClose() {
			// The caller stopped reading while the control stream was still live.
			_ = control.Close()
		}
	}()

	// Register the bounded relay before requesting the first group.
	relay, err := frametrack.NewRelay(ctx, 4)
	if err != nil {
		return err
	}
	subscription, releaseSubscription, err := st.GetFrameSubscriptions().Register(ctx, ms, relay.ReadGroup)
	if err != nil {
		return err
	}
	defer releaseSubscription()

	// Wait for the Device to admit the subscription before reporting it open.
	if err := frametrack.WriteSubscription(control, subscription); err != nil {
		return err
	}
	if err := frametrack.ReadAccepted(control); err != nil {
		return errors.Wrap(err, "the Device did not accept the frame subscription")
	}

	// The Device sends no further control data. Closure ends the track.
	go func() {
		var extra [1]byte
		_, _ = control.Read(extra[:])
		cancel(errTrackEnded)
	}()

	// Start the resource's input only after the control stream is accepted.
	if err := send(&frametrack.FrameEvent{State: &frametrack.FrameEvent_Opened{Opened: &frametrack.FrameTrackOpened{}}}); err != nil {
		return err
	}
	if recv != nil {
		go forwardInput(ctx, control, recv, cancel)
	}

	// Only this loop writes to the RPC. A blocked write holds one picture.
	for {
		// Let the relay cut any group whose pending frames fill its window.
		packet, err := relay.ReadFrame(ctx)
		if err != nil {
			if errors.Is(err, errTrackEnded) {
				return nil
			}
			return err
		}
		if err := send(&frametrack.FrameEvent{State: &frametrack.FrameEvent_Frame{Frame: packet}}); err != nil {
			return err
		}
	}
}

// resolveLink reads the peer and control protocol of the selected capability.
func (c *Client) resolveLink(ctx context.Context) (peer.ID, protocol.ID, error) {
	// Use the current Device record so re-enrolling needs no object edit.
	device, err := world.LookupObjectBody[s4wave_device.Device](ctx, c.ws, c.deviceKey)
	if err != nil {
		return "", "", err
	}
	devicePeer, err := peer.IDB58Decode(device.GetPeerId())
	if err != nil {
		return "", "", errors.Wrap(err, "frame track Device peer ID")
	}

	// Require the capability the viewer was opened on to be running.
	capabilities := device.GetCapabilities()
	index := slices.IndexFunc(capabilities, func(capability *s4wave_device.DeviceCapability) bool {
		return capability.GetId() == c.capabilityID && capability.GetKind() == c.kind
	})
	if index < 0 {
		return "", "", errors.Errorf("the Device does not publish this %s", c.kind)
	}
	capability := capabilities[index]
	if !s4wave_device.DeviceCapabilityIsSelectable(capability) {
		return "", "", errors.Errorf("the Device's %s is not running: %s", c.kind, capability.GetDetail())
	}

	// Open only the control protocol this capability declares.
	id := protocol.ID(capability.GetLink().GetProtocolId())
	if err := id.Validate(); err != nil {
		return "", "", errors.Wrap(err, "frame track control protocol")
	}
	return devicePeer, id, nil
}

// forwardInput forwards events in order while the subscription is open.
// The resource call ends the recv side when Play returns.
func forwardInput(
	ctx context.Context,
	control stream.Stream,
	recv func() (*frametrack.Input, error),
	cancel context.CancelCauseFunc,
) {
	for {
		// A viewer may stop sending input and keep reading its picture.
		input, err := recv()
		if errors.Is(err, io.EOF) || ctx.Err() != nil {
			return
		}
		if err != nil {
			cancel(err)
			return
		}

		// Forward only while the same admitted control stream is live.
		if err := frametrack.WriteInput(control, input); err != nil {
			cancel(err)
			return
		}
	}
}
