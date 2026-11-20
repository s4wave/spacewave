package frametrack

import (
	"context"
	"io"
	"net"
	"testing"
	"testing/synctest"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_inmem "github.com/aperturerobotics/controllerbus/bus/inmem"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// TestSubscriptionRegistryRefusals checks the peer and subscription ID at the
// Session bus's mounted-stream entry point.
func TestSubscriptionRegistryRefusals(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Give two Devices subscriptions and end a third on the first Device.
		registry, b := newTestSubscriptionRegistry(t)
		const device, other = peer.ID("device"), peer.ID("other")
		reads := make(chan string, 4)
		frames := make(chan *Frame, 4)
		readGroup := func(_ context.Context, group *GroupReader) {
			// Record which subscription got the reader and decode its first frame.
			reads <- group.Header().GetSubscriptionId()
			frame, err := group.ReadFrame()
			if err == nil {
				frames <- frame
			}
		}
		live, _ := registerTestSubscription(t, registry, device, readGroup)
		registerTestSubscription(t, registry, other, readGroup)
		ended, release := registerTestSubscription(t, registry, device, readGroup)
		release()
		synctest.Wait()

		// Reject a Device without subscriptions before any header can be read.
		const stranger = peer.ID("stranger")
		handler := resolveTestGroupHandler(t, b, stranger)
		if err := handler.HandleMountedStream(t.Context(), &testGroupStream{remote: stranger}); err == nil {
			t.Fatal("accepted a stream from an unsubscribed peer")
		}

		// Close groups with the wrong peer, an unknown ID, or an ended ID.
		tests := []struct {
			// name describes the reason for refusal.
			name string
			// remote is the authenticated peer.
			remote peer.ID
			// id is the subscription claimed by the header.
			id string
		}{
			{name: "wrong peer", remote: other, id: live.GetSubscriptionId()},
			{name: "unknown ID", remote: device, id: "unknown"},
			{name: "ended ID", remote: device, id: ended.GetSubscriptionId()},
		}
		for _, test := range tests {
			// Drive the resolved handler through header parsing before checking the reader.
			handler := resolveTestGroupHandler(t, b, test.remote)
			conn := openTestIncomingGroup(t, handler, test.remote)
			packet := stream_packet.NewSession(conn, maxGroupMessageSize)
			if err := packet.SendMsg(testGroupHeader(test.id)); err != nil {
				t.Fatalf("%s header write: %v", test.name, err)
			}
			synctest.Wait()
			if len(reads) != 0 {
				t.Fatalf("%s reached a subscription's reader", test.name)
			}
			if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
				t.Fatalf("%s stream read returned %v, want EOF", test.name, err)
			}
		}

		// Deliver a frame when both the live ID and the Device match.
		handler = resolveTestGroupHandler(t, b, device)
		conn := openTestIncomingGroup(t, handler, device)
		frame := &Frame{Keyframe: true, Data: []byte{1}}
		packet := stream_packet.NewSession(conn, maxGroupMessageSize)
		if err := packet.SendMsgs(testGroupHeader(live.GetSubscriptionId()), frame); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if len(reads) != 1 || <-reads != live.GetSubscriptionId() {
			t.Fatal("live group did not reach its subscription")
		}
		if len(frames) != 1 || !(<-frames).EqualVT(frame) {
			t.Fatal("subscription did not receive the group's frame")
		}
	})
}

// TestSubscriptionRegistryClosesReads verifies shutdown before and after a header.
func TestSubscriptionRegistryClosesReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Keep a second subscription live to distinguish peer and viewer lifetimes.
		registry, b := newTestSubscriptionRegistry(t)
		const device = peer.ID("device")
		stopped := make(chan error, 1)
		sub, release := registerTestSubscription(t, registry, device, func(ctx context.Context, group *GroupReader) {
			// Wait for a frame until subscription release closes the stream.
			_, err := group.ReadFrame()
			if err != nil {
				stopped <- ctx.Err()
			}
		})
		_, releaseLast := registerTestSubscription(t, registry, device, func(context.Context, *GroupReader) {})
		handler := resolveTestGroupHandler(t, b, device)

		// Leave one group waiting for a frame and another waiting for its header.
		admitted := openTestIncomingGroup(t, handler, device)
		packet := stream_packet.NewSession(admitted, maxGroupMessageSize)
		if err := packet.SendMsg(testGroupHeader(sub.GetSubscriptionId())); err != nil {
			t.Fatal(err)
		}
		pending := openTestIncomingGroup(t, handler, device)
		synctest.Wait()

		// Ending one subscription interrupts its own admitted reader.
		release()
		synctest.Wait()
		if len(stopped) != 1 || <-stopped != context.Canceled {
			t.Fatal("subscription release did not interrupt its group reader")
		}

		// The last release interrupts reads waiting for an unidentified header.
		releaseLast()
		synctest.Wait()
		if _, err := pending.Read(make([]byte, 1)); err != io.EOF {
			t.Fatalf("unfinished header stream read returned %v, want EOF", err)
		}
	})
}

// newTestSubscriptionRegistry installs a receiver on an in-memory Session bus.
func newTestSubscriptionRegistry(t *testing.T) (*SubscriptionRegistry, bus.Bus) {
	// Run the actual directive controller for this viewer peer.
	t.Helper()
	registry := NewSubscriptionRegistry(peer.ID("viewer"))
	le := logrus.NewEntry(logrus.New())
	b := bus_inmem.NewBus(directive_controller.NewController(t.Context(), le))

	// Keep the resolver installed for the test's stream lifetime.
	release, err := b.AddHandler(registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return registry, b
}

// registerTestSubscription binds a viewer to an authenticated test control stream.
func registerTestSubscription(
	t *testing.T,
	registry *SubscriptionRegistry,
	remote peer.ID,
	readGroup func(context.Context, *GroupReader),
) (*Subscription, func()) {
	t.Helper()
	sub, release, err := registry.Register(t.Context(), &testGroupStream{remote: remote}, readGroup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return sub, release
}

// resolveTestGroupHandler resolves the same directive as the link controller.
func resolveTestGroupHandler(t *testing.T, b bus.Bus, remote peer.ID) link.MountedStreamHandler {
	// Hold the directive reference while driving its handler.
	t.Helper()
	dir := link.NewHandleMountedStream(GroupProtocolID, peer.ID("viewer"), remote)
	handler, _, ref, err := bus.ExecWaitValue[link.MountedStreamHandler](t.Context(), b, dir, nil, nil, nil)
	if ref != nil {
		t.Cleanup(ref.Release)
	}
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// openTestIncomingGroup mounts one end of a pipe with the Device's peer ID.
func openTestIncomingGroup(t *testing.T, handler link.MountedStreamHandler, remote peer.ID) net.Conn {
	// Close both pipe ends if a failing assertion stops the test.
	t.Helper()
	local, device := net.Pipe()
	t.Cleanup(func() { _ = device.Close() })
	t.Cleanup(func() { _ = local.Close() })

	// Hand the incoming stream to the receiver and keep its remote end to write.
	if err := handler.HandleMountedStream(t.Context(), &testGroupStream{remote: remote, strm: local}); err != nil {
		t.Fatal(err)
	}
	return device
}

// testGroupHeader creates a valid header for the supplied subscription ID.
func testGroupHeader(id string) *GroupHeader {
	return &GroupHeader{
		SubscriptionId: id, GroupSequence: 1, Codec: Codec_CODEC_JPEG, Width: 640, Height: 360,
	}
}

// testGroupStream carries the authenticated peer and a controllable stream.
type testGroupStream struct {
	// MountedStream supplies the metadata this test does not exercise.
	link.MountedStream
	// remote is the Device identity established by the link.
	remote peer.ID
	// strm carries the peer's bytes.
	strm stream.Stream
}

// GetPeerID returns the authenticated Device.
func (m *testGroupStream) GetPeerID() peer.ID { return m.remote }

// GetStream returns the stream the handler can read and close.
func (m *testGroupStream) GetStream() stream.Stream { return m.strm }

// GetOpenOpts marks the test stream as reliable.
func (m *testGroupStream) GetOpenOpts() stream.OpenOpts { return stream.OpenOpts{} }

// _ is a type assertion.
var _ link.MountedStream = (*testGroupStream)(nil)
