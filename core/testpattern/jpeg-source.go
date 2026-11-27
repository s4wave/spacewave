package testpattern

import (
	"context"
	"image"
	"sync"

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

	// mtx guards the pointer written by admitted viewers.
	mtx sync.Mutex
	// pointer is the latest position to draw in frame pixels.
	pointer image.Point
}

// NewJPEGSource starts the clock for a 640 by 360 JPEG test pattern.
func NewJPEGSource() *JPEGSource {
	return &JPEGSource{
		pattern: newPattern(),
		header: &frametrack.GroupHeader{
			Codec:  frametrack.Codec_CODEC_JPEG,
			Width:  Width,
			Height: Height,
		},
		frameClock: newFrameClock(),
		pointer:    image.Pt(-1, -1),
	}
}

// CanRequestKeyframe reports false because every picture is a keyframe.
func (s *JPEGSource) CanRequestKeyframe() bool {
	return false
}

// SetPointer sets the position for the next JPEG. It can race with ReadFrame.
func (s *JPEGSource) SetPointer(pointer image.Point) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.pointer = pointer
}

// ReadFrame waits for the next tick and returns one complete JPEG.
func (s *JPEGSource) ReadFrame(ctx context.Context) (*frametrack.GroupHeader, *frametrack.Frame, error) {
	// Wait for the clock, shutdown, or the reader's cancellation.
	captured, err := s.next(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Snapshot input at capture time. Drawing and encoding leave input free.
	s.mtx.Lock()
	pointer := s.pointer
	s.mtx.Unlock()
	s.pattern.SetPointer(pointer)
	s.pattern.Draw()

	// Encode the current picture into bytes owned by this frame.
	encoded, err := s.pattern.Encode()
	if err != nil {
		return nil, nil, err
	}

	// Publish the JPEG and its capture timestamp together.
	return s.header, &frametrack.Frame{
		CapturedAt: timestamp.New(captured),
		Keyframe:   true,
		Data:       encoded,
	}, nil
}

// RequestKeyframe leaves the capture cadence in control of the next keyframe.
func (s *JPEGSource) RequestKeyframe() {}

// _ is a type assertion
var _ frametrack.Source = (*JPEGSource)(nil)
