package testpattern

import (
	"bytes"
	"context"
	"io"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/core/frametrack"
)

// jpegFrameInterval sets the test pattern cadence to 30 frames per second.
const jpegFrameInterval = time.Second / 30

// JPEGSource produces independently decodable pictures of the test pattern.
// One caller reads frames in capture order.
type JPEGSource struct {
	// pattern renders into a reusable image owned by the reader.
	pattern *Pattern
	// header holds the codec and dimensions shared by all its frames.
	header *frametrack.GroupHeader
	// ticker sets the capture cadence and drops ticks the reader misses.
	ticker *time.Ticker
	// done closes when the source stops.
	done <-chan struct{}
	// cancel closes done, including when a read is waiting for a tick.
	cancel context.CancelFunc
}

// NewJPEGSource starts the clock for a 640 by 360 JPEG test pattern.
func NewJPEGSource() *JPEGSource {
	// Give readers a shutdown signal independent of their own context.
	ctx, cancel := context.WithCancel(context.Background())
	return &JPEGSource{
		pattern: New(),
		header: &frametrack.GroupHeader{
			Codec:  frametrack.Codec_CODEC_JPEG,
			Width:  Width,
			Height: Height,
		},
		ticker: time.NewTicker(jpegFrameInterval), //gostyle:timer deadline scheduled frame capture
		done:   ctx.Done(),
		cancel: cancel,
	}
}

// CanRequestKeyframe reports false because every picture is a keyframe.
func (s *JPEGSource) CanRequestKeyframe() bool {
	return false
}

// ReadFrame waits for the next tick and returns one complete JPEG.
func (s *JPEGSource) ReadFrame(ctx context.Context) (*frametrack.GroupHeader, *frametrack.Frame, error) {
	// Wait for the clock, shutdown, or the reader's cancellation.
	var captured time.Time
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-s.done:
		return nil, nil, io.EOF
	case captured = <-s.ticker.C:
	}

	// Encode the current picture into bytes owned by this frame.
	var encoded bytes.Buffer
	if err := s.pattern.Encode(&encoded); err != nil {
		return nil, nil, err
	}
	s.pattern.Draw()

	// Publish the JPEG and its capture timestamp together.
	return s.header, &frametrack.Frame{
		CapturedAt: timestamp.New(captured),
		Keyframe:   true,
		Data:       encoded.Bytes(),
	}, nil
}

// RequestKeyframe leaves the capture cadence in control of the next keyframe.
func (s *JPEGSource) RequestKeyframe() {}

// Close stops the clock and releases readers waiting for a tick.
func (s *JPEGSource) Close() error {
	s.cancel()
	s.ticker.Stop()
	return nil
}

// _ is a type assertion.
var _ frametrack.Source = (*JPEGSource)(nil)
