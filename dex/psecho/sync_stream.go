package psecho

import (
	"github.com/aperturerobotics/bifrost/stream"
	"github.com/aperturerobotics/bifrost/stream/packet"
)

// syncStream is a sync session stream.
type syncStream struct {
	*stream_packet.Session
}

// newSyncStream constructs a new sync stream.
func newSyncStream(strm stream.Stream) *syncStream {
	return &syncStream{Session: stream_packet.NewSession(strm, uint32(maxMessageSize))}
}

// sendSyncMessage writes a sync message to the stream.
func (s *syncStream) sendSyncMessage(msg *SyncMessage) error {
	return s.Session.SendMsg(msg)
}

// readSyncMessage reads a sync message from the stream.
func (s *syncStream) readSyncMessage(msg *SyncMessage) error {
	return s.Session.RecvMsg(msg)
}
