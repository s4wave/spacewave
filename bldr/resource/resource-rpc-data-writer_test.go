package resource

import (
	"bytes"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
)

// msgSendRecorder is a ResourceRpcStream that keeps the bytes of each message.
type msgSendRecorder struct {
	ResourceRpcStream
	// frames holds the marshaled messages in send order.
	frames [][]byte
}

// MsgSend marshals the message the way a frame encoder would.
func (r *msgSendRecorder) MsgSend(msg srpc.Message) error {
	data, err := msg.MarshalVT()
	if err != nil {
		return err
	}
	r.frames = append(r.frames, data)
	return nil
}

// TestResourceRpcDataWriterFrameMatchesGeneratedPacket checks that the nested
// data frame has the bytes of a ResourceRpcPacket that holds the marshaled
// SRPC packet, for payloads on both sides of each varint length boundary.
func TestResourceRpcDataWriterFrameMatchesGeneratedPacket(t *testing.T) {
	for _, size := range []int{1, 127, 128, 16383, 16384, 100000} {
		pkt := srpc.NewCallDataPacket(bytes.Repeat([]byte{7}, size), false, false, nil)

		// Write the packet through the data writer.
		rec := &msgSendRecorder{}
		if err := (&resourceRpcDataWriter{stream: rec}).WritePacket(pkt); err != nil {
			t.Fatal(err)
		}

		// Compare with the generated encoding of the same frame.
		inner, err := pkt.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		want, err := (&ResourceRpcPacket{Body: &ResourceRpcPacket_Data{Data: inner}}).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if len(rec.frames) != 1 || !bytes.Equal(rec.frames[0], want) {
			t.Fatalf("size %d: data frame differs from the generated ResourceRpcPacket", size)
		}
	}
}
