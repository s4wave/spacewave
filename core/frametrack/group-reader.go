package frametrack

import (
	"io"

	"github.com/pkg/errors"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// maxGroupMessageSize bounds the encoded header and each frame at 16 MiB.
const maxGroupMessageSize = 16 << 20

// GroupReader reads the header and complete frames on one group stream.
type GroupReader struct {
	// session decodes length-prefixed messages from the stream.
	session *stream_packet.Session
	// header describes the group and stays immutable after construction.
	header *GroupHeader
	// started records whether the group's first frame has been read.
	started bool
}

// NewGroupReader reads and validates the group header.
// The caller owns strm and checks the header's subscription against the peer
// before reading any frames.
func NewGroupReader(strm io.ReadWriteCloser) (*GroupReader, error) {
	// Decode the first packet as the group header.
	session := stream_packet.NewSession(strm, maxGroupMessageSize)
	header := new(GroupHeader)
	if err := session.RecvMsg(header); err != nil {
		return nil, err
	}

	// Require the identity and format the decoder needs for this group.
	if header.GetSubscriptionId() == "" || header.GetGroupSequence() == 0 ||
		header.GetWidth() == 0 || header.GetHeight() == 0 {
		return nil, errors.New("frame group is missing its identity or size")
	}
	switch header.GetCodec() {
	case Codec_CODEC_JPEG:
	case Codec_CODEC_H264_AVC:
		if len(header.GetDecoderDescription()) == 0 {
			return nil, errors.New("H.264 frame group is missing its decoder description")
		}
	default:
		return nil, errors.New("unsupported frame group codec")
	}

	return &GroupReader{session: session, header: header}, nil
}

// Header returns the immutable group metadata.
func (r *GroupReader) Header() *GroupHeader {
	return r.header
}

// ReadFrame reads the next picture. The caller stops reading at the first error.
func (r *GroupReader) ReadFrame() (*Frame, error) {
	// Decode one complete frame.
	frame := new(Frame)
	if err := r.session.RecvMsg(frame); err != nil {
		return nil, err
	}

	// Accept a keyframe at the start and deltas only within an H.264 group.
	if frame.GetKeyframe() == r.started || (r.started && r.header.GetCodec() == Codec_CODEC_JPEG) {
		return nil, errors.New("frame violates the group's keyframe order")
	}
	r.started = true
	return frame, nil
}
