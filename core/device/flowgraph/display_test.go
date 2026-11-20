//go:build !js

package device_flowgraph

import (
	"bytes"
	"context"
	"image/jpeg"
	"testing"

	"github.com/s4wave/spacewave/core/frametrack"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
)

// checkDisplaySubscription sends input on the admitted viewer's real control
// stream and watches the resulting JPEGs. The caller revokes the live track.
func checkDisplaySubscription(
	t *testing.T,
	ctx context.Context,
	device peer.ID,
	viewer *transport.SessionTransport,
	outsider *transport.SessionTransport,
	id protocol.ID,
) stream.Stream {
	// Register the viewer before asking the Device for the first group.
	t.Helper()
	ms := openFrameControl(t, ctx, device, viewer, id)
	control := ms.GetStream()
	pictures := make(chan *frametrack.Frame, 1)
	subscription, release, err := viewer.GetFrameSubscriptions().Register(ctx, ms, func(_ context.Context, group *frametrack.GroupReader) {
		// The JPEG groups each carry one picture. A busy test can skip one.
		frame, err := group.ReadFrame()
		if err != nil {
			return
		}
		select {
		case pictures <- frame:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	if err := frametrack.WriteSubscription(control, subscription); err != nil {
		t.Fatal(err)
	}

	// A display accepts key transitions before the next pointer position.
	for _, pressed := range []bool{true, false} {
		key := &frametrack.Input_Key{Key: &frametrack.KeyInput{Code: "KeyA", Key: "a", Pressed: pressed}}
		if err := frametrack.WriteInput(control, &frametrack.Input{Event: key}); err != nil {
			t.Fatal(err)
		}
	}
	pointer := &frametrack.Input{Event: &frametrack.Input_Pointer{Pointer: &frametrack.PointerInput{X: 0.5, Y: 0.25}}}
	if err := frametrack.WriteInput(control, pointer); err != nil {
		t.Fatal(err)
	}

	// A linked outsider cannot attach a subscription or move the pointer.
	refused := openFrameControl(t, ctx, device, outsider, id).GetStream()
	if frametrack.WriteSubscription(refused, &frametrack.Subscription{SubscriptionId: "display-outsider"}) == nil {
		// Refusal can close the stream before the input reaches the Device.
		_ = frametrack.WriteInput(refused, &frametrack.Input{
			Event: &frametrack.Input_Pointer{Pointer: &frametrack.PointerInput{X: 0.75, Y: 0.25}},
		})
	}
	checkFrameControlClosed(t, refused)

	// Wait for a captured frame whose white cross is at the viewer's position.
	for {
		select {
		case <-ctx.Done():
			t.Fatal("display never echoed the admitted viewer's pointer")
		case frame := <-pictures:
			// Require a complete JPEG with the pointer above the dimmer bars.
			picture, err := jpeg.Decode(bytes.NewReader(frame.GetData()))
			if err != nil {
				t.Fatal(err)
			}
			r, g, b, _ := picture.At(320, 90).RGBA()
			if min(r, g, b) > 0xf000 {
				return control
			}
		}
	}
}
