package testpattern

import (
	"context"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/core/frametrack"
)

// JPEGSource produces independently decodable pictures of the test pattern.
// One caller reads frames in capture order.
type JPEGSource struct {
	// pattern renders into a reusable image owned by the reader.
	pattern *Pattern
	// header holds the codec and dimensions shared by all its frames.
	header *frametrack.GroupHeader
	// frameClock paces capture and stops the reader when the source closes.
	frameClock
}

// NewJPEGSource starts the clock for a 640 by 360 JPEG test pattern.
func NewJPEGSource() *JPEGSource {
	return &JPEGSource{
		pattern: New(),
		header: &frametrack.GroupHeader{
			Codec:  frametrack.Codec_CODEC_JPEG,
			Width:  Width,
			Height: Height,
		},
		frameClock: newFrameClock(),
	}
}

// CanRequestKeyframe reports false because every picture is a keyframe.
func (s *JPEGSource) CanRequestKeyframe() bool {
	return false
}

// ReadFrame waits for the next tick and returns one complete JPEG.
func (s *JPEGSource) ReadFrame(ctx context.Context) (*frametrack.GroupHeader, *frametrack.Frame, error) {
	// Wait for the clock, shutdown, or the reader's cancellation.
	captured, err := s.next(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Encode the current picture into bytes owned by this frame.
	encoded, err := s.pattern.Encode()
	if err != nil {
		return nil, nil, err
	}
	s.pattern.Draw()

	// Publish the JPEG and its capture timestamp together.
	return s.header, &frametrack.Frame{
		CapturedAt: timestamp.New(captured),
		Keyframe:   true,
		Data:       encoded,
	}, nil
}

// RequestKeyframe leaves the capture cadence in control of the next keyframe.
func (s *JPEGSource) RequestKeyframe() {}

// _ is a type assertion.
var _ frametrack.Source = (*JPEGSource)(nil)
