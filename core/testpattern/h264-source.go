package testpattern

import (
	"context"
	_ "embed"
	"encoding/binary"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/core/frametrack"
)

// barsClip holds the immutable, video-only FLV fixture.
//
//go:embed bars.flv
var barsClip []byte

// firstTag is the offset past the nine-byte FLV header and first tag size.
const firstTag = 13

// H264Source loops the embedded bars at 30 frames per second in decode order.
// One caller reads frames in capture order.
type H264Source struct {
	// header holds the common AVC configuration for each keyframe.
	header *frametrack.GroupHeader
	// start is the first picture after the AVC configuration.
	start int
	// offset is the next video tag to read.
	offset int
	// frameClock paces capture and stops the reader when the source closes.
	frameClock
}

// NewH264Source starts the clock for the embedded 640 by 360 bars clip.
func NewH264Source() *H264Source {
	// Read the AVC sequence header before the first coded picture.
	_, description, start := readBarsTag(firstTag)
	return &H264Source{
		header: &frametrack.GroupHeader{
			Codec:              frametrack.Codec_CODEC_H264_AVC,
			Width:              Width,
			Height:             Height,
			DecoderDescription: description,
		},
		start:      start,
		offset:     start,
		frameClock: newFrameClock(),
	}
}

// CanRequestKeyframe reports false because the clip is already encoded.
func (s *H264Source) CanRequestKeyframe() bool {
	return false
}

// ReadFrame waits for the next tick and returns the next immutable AVC access unit.
func (s *H264Source) ReadFrame(ctx context.Context) (*frametrack.GroupHeader, *frametrack.Frame, error) {
	// Pace playback with the same clock as the JPEG test pattern.
	captured, err := s.next(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Advance through the clip and resume at its first keyframe on wrap.
	videoHeader, data, next := readBarsTag(s.offset)
	if next == len(barsClip) {
		next = s.start
	}
	s.offset = next

	// Carry the AVC description on the keyframes that open the groups.
	keyframe := videoHeader>>4 == 1
	var header *frametrack.GroupHeader
	if keyframe {
		header = s.header
	}
	return header, &frametrack.Frame{
		CapturedAt: timestamp.New(captured),
		Keyframe:   keyframe,
		Data:       data,
	}, nil
}

// RequestKeyframe leaves the embedded clip's keyframe cadence in control.
func (s *H264Source) RequestKeyframe() {}

// readBarsTag reads the next tag in the fixed video-only fixture. Each tag
// starts with its 11-byte FLV tag header and five-byte AVC video header and
// ends with a four-byte previous-tag size.
func readBarsTag(offset int) (videoHeader byte, payload []byte, next int) {
	size := int(binary.BigEndian.Uint32(barsClip[offset+1:offset+5]) >> 8)
	end := offset + 11 + size
	return barsClip[offset+11], barsClip[offset+16 : end : end], end + 4
}

// _ is a type assertion.
var _ frametrack.Source = (*H264Source)(nil)
