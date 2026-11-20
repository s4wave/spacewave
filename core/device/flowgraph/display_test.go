//go:build !js

package device_flowgraph

import (
	"bytes"
	"context"
	"image/jpeg"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/core/frametrack"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	s4wave_display "github.com/s4wave/spacewave/sdk/display"
	s4wave_display_world "github.com/s4wave/spacewave/sdk/display/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// openDisplay creates a Display object and connects through its typed RPC.
func openDisplay(
	t *testing.T,
	ctx context.Context,
	engine world.Engine,
	b bus.Bus,
	session peer.ID,
	capabilityID string,
	key string,
) s4wave_display.SRPCDisplayResourceService_ConnectDisplayClient {
	// Store the object through the same operation the Device table uses.
	t.Helper()
	commit(t, ctx, engine, func(tx world.Tx) error {
		_, _, err := tx.ApplyWorldOp(ctx, &s4wave_display.CreateDisplayOp{
			ObjectKey: key, Name: "Display", DeviceObjectKey: testSelfKey,
			CapabilityId: capabilityID, Timestamp: timestamppb.Now(),
		}, "")
		return err
	})

	// Access the Display from the viewer's Session and start the live RPC.
	invoker, release, err := s4wave_display_world.DisplayType.GetFactory()(
		objecttype.WithSessionPeerID(ctx, session), nil, b, engine, world.NewEngineWorldState(engine, false), key,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	client := s4wave_display.NewSRPCDisplayResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker))),
	)
	frames, err := client.ConnectDisplay(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = frames.Close() })
	if opened, err := frames.Recv(); err != nil || opened.GetOpened() == nil {
		t.Fatalf("display opened with %v, error %v", opened, err)
	}
	return frames
}

// checkDisplayJPEG tests input, account admission, and the next JPEG draw.
func checkDisplayJPEG(
	t *testing.T,
	ctx context.Context,
	device peer.ID,
	outsider *transport.SessionTransport,
	id protocol.ID,
	frames s4wave_display.SRPCDisplayResourceService_ConnectDisplayClient,
) {
	// Key transitions and an independent pointer event use one subscription.
	t.Helper()
	for _, pressed := range []bool{true, false} {
		key := &frametrack.Input_Key{Key: &frametrack.KeyInput{Code: "KeyA", Key: "a", Pressed: pressed}}
		if err := frames.Send(&frametrack.Input{Event: key}); err != nil {
			t.Fatal(err)
		}
	}
	pointer := &frametrack.Input{Event: &frametrack.Input_Pointer{Pointer: &frametrack.PointerInput{X: 0.5, Y: 0.25}}}
	if err := frames.Send(pointer); err != nil {
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

	// Wait for a complete JPEG that draws the cross over its dimmer background.
	for {
		// Read through the registered Display resource and its group relay.
		response, err := frames.Recv()
		if err != nil {
			t.Fatal(err)
		}
		packet := response.GetFrame()
		if packet.GetHeader().GetCodec() != frametrack.Codec_CODEC_JPEG {
			t.Fatalf("display sent a non-JPEG packet: %v", packet.GetHeader())
		}
		picture, err := jpeg.Decode(bytes.NewReader(packet.GetFrame().GetData()))
		if err != nil {
			t.Fatal(err)
		}
		r, g, b, _ := picture.At(320, 90).RGBA()
		if min(r, g, b) > 0xf000 {
			return
		}
	}
}

// checkDisplayAVC requires an IDR and its first two dependent pictures.
func checkDisplayAVC(t *testing.T, frames s4wave_display.SRPCDisplayResourceService_ConnectDisplayClient) {
	// Open the decoder from the first complete group's header.
	t.Helper()
	for n := range 3 {
		// Read the access units in decode order from the real resource.
		response, err := frames.Recv()
		if err != nil {
			t.Fatal(err)
		}
		packet := response.GetFrame()
		header, frame := packet.GetHeader(), packet.GetFrame()

		// The keyframe starts playback with the AVC description.
		if n == 0 && (header.GetGroupSequence() == 0 ||
			header.GetCodec() != frametrack.Codec_CODEC_H264_AVC || len(header.GetDecoderDescription()) < 7) {
			t.Fatalf("AVC opening header = %v", header)
		}

		// The following access units depend on the opening keyframe.
		if n != 0 && header != nil {
			t.Fatalf("AVC delta repeated the group header: %v", header)
		}
		if frame.GetKeyframe() != (n == 0) || len(frame.GetData()) == 0 {
			t.Fatalf("AVC picture %d has keyframe %v and %d bytes", n, frame.GetKeyframe(), len(frame.GetData()))
		}
	}
}
