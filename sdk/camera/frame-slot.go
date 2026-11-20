package s4wave_camera

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/core/frametrack"
)

// frameSlot holds the newest picture the caller has not yet taken. A picture
// that arrives while one waits replaces it, so a caller that falls behind skips
// pictures instead of queueing them. That is correct only while every picture
// decodes alone, as each JPEG does.
type frameSlot struct {
	// bcast guards pending and newest and wakes the taker.
	bcast broadcast.Broadcast
	// pending is the newest picture not yet taken, or nil.
	pending *WatchFramesFrame
	// newest is the group sequence of the newest picture put in the slot.
	newest uint64
}

// readGroup puts each picture of a group in the slot until the group ends.
// Groups are read concurrently, so a picture from a group older than the
// newest one seen is dropped rather than shown after it.
func (s *frameSlot) readGroup(_ context.Context, group *frametrack.GroupReader) {
	header := group.Header()
	for {
		// Read the next complete picture; any error ends the group.
		frame, err := group.ReadFrame()
		if err != nil {
			return
		}

		// Replace the waiting picture unless a newer group has already put one.
		s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if header.GetGroupSequence() < s.newest {
				return
			}
			s.newest = header.GetGroupSequence()
			s.pending = &WatchFramesFrame{Header: header, Frame: frame}
			broadcast()
		})
	}
}

// take waits for a picture and removes it from the slot. It returns the
// context's cause when ctx ends first.
func (s *frameSlot) take(ctx context.Context) (*WatchFramesFrame, error) {
	for {
		// Take the waiting picture with the wait channel for the next one.
		var frame *WatchFramesFrame
		var waitCh <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			frame, s.pending = s.pending, nil
			waitCh = getWaitCh()
		})
		if frame != nil {
			return frame, nil
		}

		// Wait for the next picture.
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-waitCh:
		}
	}
}
