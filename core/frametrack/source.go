package frametrack

import "context"

// Source produces one rendition in capture order for all viewers of a track.
type Source interface {
	// CanRequestKeyframe reports whether the source can encode a new keyframe
	// on demand.
	CanRequestKeyframe() bool
	// ReadFrame returns the next picture or io.EOF when the source ends.
	// Each keyframe includes its codec, size and decoder description in header.
	// The header can be nil for a delta. The track assigns group identity.
	// The source leaves the returned message and its bytes immutable.
	// Cancellation interrupts a pending read.
	ReadFrame(ctx context.Context) (header *GroupHeader, frame *Frame, err error)
	// RequestKeyframe asks the source to make its next frame a keyframe.
	// Calls return without waiting for the encoder and can race with reads
	// and shutdown. Multiple requests may share the same keyframe.
	RequestKeyframe()
	// Close stops the source and releases its encoder or device.
	Close() error
}
