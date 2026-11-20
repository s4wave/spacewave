package frametrack

import (
	"net"
	"testing"
	"testing/synctest"

	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// TestRelayPreservesDecodablePrefix holds the consumer while the relay fills.
func TestRelayPreservesDecodablePrefix(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Let one group fill its window before the consumer reads anything.
		relay, err := NewRelay(t.Context(), 2)
		if err != nil {
			t.Fatal(err)
		}
		readRelayGroup(t, relay, 3, 3, 4, 5)

		// An older group and a second stream for this group cannot displace it.
		readRelayGroup(t, relay, 2, 2)
		readRelayGroup(t, relay, 3, 9)
		checkRelayFrame(t, relay, 1, 3, true)
		checkRelayFrame(t, relay, 1, 4, false)

		// A newer keyframe resumes playback after the omitted tail.
		readRelayGroup(t, relay, 4, 6)
		checkRelayFrame(t, relay, 2, 6, true)
	})
}

// readRelayGroup presents a complete group or lets the relay end its stream.
func readRelayGroup(t *testing.T, relay *Relay, sequence uint64, frames ...byte) {
	// Give the relay the same packet reader used for a peer's incoming group.
	t.Helper()
	receiver, sender := net.Pipe()
	defer receiver.Close()
	go func() {
		// Write the group until a receiver closes its half.
		defer sender.Close()
		stream := stream_packet.NewSession(sender, maxGroupMessageSize)
		if err := stream.SendMsg(&GroupHeader{
			SubscriptionId: "display", GroupSequence: sequence,
			Width: 640, Height: 360, Codec: Codec_CODEC_H264_AVC, DecoderDescription: []byte{1, 2, 3},
		}); err != nil {
			return
		}
		for i, dat := range frames {
			if err := stream.SendMsg(&Frame{Keyframe: i == 0, Data: []byte{dat}}); err != nil {
				return
			}
		}
	}()

	// Read the header and hand the stream to the relay's group consumer.
	group, err := NewGroupReader(receiver)
	if err != nil {
		t.Fatal(err)
	}
	relay.ReadGroup(t.Context(), group)
}

// checkRelayFrame checks group identity, frame order, and the keyframe flag.
func checkRelayFrame(t *testing.T, relay *Relay, sequence uint64, data byte, keyframe bool) {
	// Take one waiting picture through the same API used by the resource.
	t.Helper()
	packet, err := relay.ReadFrame(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	// Only the intended group's pictures may reach the viewer.
	header, frame := packet.GetHeader(), packet.GetFrame()
	if keyframe && (header.GetGroupSequence() != sequence || header.GetSubscriptionId() != "display") {
		t.Fatalf("opening header = %v, want display group %d", header, sequence)
	}
	if !keyframe && header != nil {
		t.Fatalf("a delta repeated the group header: %v", header)
	}
	if frame.GetKeyframe() != keyframe || len(frame.GetData()) != 1 || frame.GetData()[0] != data {
		t.Fatalf("frame = %v, want data %d and keyframe %v", frame, data, keyframe)
	}
}
