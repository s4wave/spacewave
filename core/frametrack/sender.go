// Package frametrack sends live pictures as independently decodable groups.
// Each control or group packet begins with the protobuf payload's byte count
// as a little-endian uint32.
package frametrack

import (
	"context"
	"io"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// Sender delivers one subscription with a separate stream for each group.
type Sender struct {
	// ctx ends the subscription and all of its group writers.
	ctx context.Context
	// cancel ends the subscription independently of its parent.
	cancel context.CancelFunc
	// subscriptionID is echoed in every group header.
	subscriptionID string
	// window bounds each group's unwritten frames, including the active write.
	window int
	// open opens the stream for a group.
	open OpenGroupStream
	// requestKeyframe signals an encoder after a drop. It can be nil.
	requestKeyframe func()

	// bcast guards the current group, next sequence and pending request.
	bcast broadcast.Broadcast
	// group is the newest group, which may still be draining its prefix.
	group *sendGroup
	// sequence is the last assigned group number, including failed opens.
	sequence uint64
	// keyRequested records whether the source has been asked for a keyframe.
	keyRequested bool
}

// sendGroup holds a group's unwritten frames under the sender's broadcast lock.
type sendGroup struct {
	// cancel interrupts the writer when a newer keyframe replaces the group.
	cancel context.CancelFunc
	// frames is the ring of unwritten frames.
	frames []*Frame
	// head is the index of the oldest unwritten frame.
	head int
	// count includes the frame currently being written.
	count int
	// sealed prevents further deltas from joining this group.
	sealed bool
}

// NewSender creates a sender for one subscription.
// open must return a stream whose Close releases a blocked Write.
// The opener stops when its context ends.
// requestKeyframe must be safe for concurrent calls and return without waiting
// for the encoder. Pass nil for a source that supplies its own keyframes.
func NewSender(
	ctx context.Context,
	subscriptionID string,
	window int,
	open OpenGroupStream,
	requestKeyframe func(),
) (*Sender, error) {
	// Require the information needed to begin a group.
	if subscriptionID == "" || window < 1 || open == nil {
		return nil, errors.New("frame sender requires a subscription ID, positive window, and stream opener")
	}

	// Derive a lifetime for the subscription and its groups.
	ctx, cancel := context.WithCancel(ctx)
	return &Sender{
		ctx:             ctx,
		cancel:          cancel,
		subscriptionID:  subscriptionID,
		window:          window,
		open:            open,
		requestKeyframe: requestKeyframe,
	}, nil
}

// Send queues a frame in capture order without waiting for stream I/O.
// A keyframe supplies the group format in header. The sender assigns the
// subscription ID and sequence. Deltas can pass a nil header.
// The caller keeps the frame and the decoder description immutable after Send.
// Call Send with frames from one source in capture order.
// A true result means the sender queued the frame for writing.
func (s *Sender) Send(header *GroupHeader, frame *Frame) bool {
	// Decide where the frame belongs and seal the group on the first gap.
	var previous *sendGroup
	var group *sendGroup
	var groupCtx context.Context
	var groupHeader GroupHeader
	var queued, request bool
	s.bcast.HoldLock(func(wake func(), _ func() <-chan struct{}) {
		switch {
		case s.ctx.Err() != nil:
			return
		case frame.GetKeyframe():
			// Publish a new group with the keyframe at the head of its queue.
			previous = s.group
			var cancel context.CancelFunc
			groupCtx, cancel = context.WithCancel(s.ctx)
			group = &sendGroup{cancel: cancel, frames: make([]*Frame, s.window), count: 1}
			group.frames[0] = frame
			s.sequence++
			groupHeader = GroupHeader{
				SubscriptionId:     s.subscriptionID,
				GroupSequence:      s.sequence,
				Codec:              header.GetCodec(),
				Width:              header.GetWidth(),
				Height:             header.GetHeight(),
				DecoderDescription: header.GetDecoderDescription(),
			}
			s.group = group
			s.keyRequested = false
			queued = true
		case s.group != nil && !s.group.sealed && s.group.count < s.window:
			// Append the delta behind all earlier unwritten frames.
			g := s.group
			g.frames[(g.head+g.count)%s.window] = frame
			g.count++
			queued = true
			wake()
		default:
			// Keep the queued prefix and end intake until the next keyframe.
			request = s.sealGroup()
			wake()
		}
	})

	// Let the new group's writer proceed independently of the preceding stream.
	if previous != nil {
		previous.cancel()
	}
	if group != nil {
		go s.runGroup(groupCtx, group, &groupHeader)
	}

	// Signal the encoder outside the sender lock.
	if request {
		s.requestKeyframe()
	}
	return queued
}

// Close cancels the subscription and interrupts its group writers.
func (s *Sender) Close() error {
	s.cancel()
	return nil
}

// runGroup opens and writes one group until the queue drains or the group is
// replaced. An open or write failure seals the group before requesting a keyframe.
func (s *Sender) runGroup(ctx context.Context, group *sendGroup, header *GroupHeader) {
	// Clear the current group on exit and request recovery after a wire failure.
	defer group.cancel()
	defer func() {
		var request bool
		s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if s.group == group {
				request = ctx.Err() == nil && !group.sealed && s.sealGroup()
				s.group = nil
			}
		})
		if request {
			s.requestKeyframe()
		}
	}()

	// Open one stream for this group and close it when its context ends.
	if ctx.Err() != nil {
		return
	}
	strm, err := s.open(ctx)
	if err != nil {
		return
	}
	stopClose := context.AfterFunc(ctx, func() {
		// Close interrupts a write on a retired group.
		_ = strm.Close()
	})
	defer func() {
		if stopClose() {
			// Close sends the FIN after the group's final write.
			_ = strm.Close()
		}
	}()

	// Write the header and successive frames on this stream.
	session := stream_packet.NewSession(strm, maxGroupMessageSize)
	if header.SizeVT() > maxGroupMessageSize || session.SendMsg(header) != nil {
		return
	}
	for {
		// Peek at the queue without freeing the slot of a blocked write.
		var frame *Frame
		var wait <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			switch {
			case s.group != group || ctx.Err() != nil:
			case group.count != 0:
				frame = group.frames[group.head]
			case !group.sealed:
				wait = getWaitCh()
			}
		})

		// A sealed group finishes once its prefix is written.
		if frame == nil {
			if wait == nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-wait:
				continue
			}
		}

		// Retire the frame only after its write succeeds.
		if frame.SizeVT() > maxGroupMessageSize || session.SendMsg(frame) != nil {
			return
		}
		s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			group.frames[group.head] = nil
			group.head = (group.head + 1) % s.window
			group.count--
		})
	}
}

// sealGroup stops intake and records a new keyframe request under s.bcast.
func (s *Sender) sealGroup() bool {
	// Close the group to later deltas while its writer drains the prefix.
	if s.group != nil {
		s.group.sealed = true
	}

	// Coalesce drops until the source produces a keyframe.
	if s.keyRequested || s.requestKeyframe == nil {
		return false
	}
	s.keyRequested = true
	return true
}

// _ is a type assertion.
var _ io.Closer = (*Sender)(nil)
