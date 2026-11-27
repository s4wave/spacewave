package frametrack

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"

	"github.com/s4wave/spacewave/net/link"
)

// Track reads one source and fans out each frame to the track's subscriptions.
type Track struct {
	// source produces the shared frames.
	source Source
	// requestKeyframe signals the shared encoder when supported.
	requestKeyframe func()

	// mtx guards subscriptions and the end of the source.
	mtx sync.Mutex
	// senders is an immutable snapshot replaced when membership changes.
	senders []*Sender
	// stopped records that Run has ended and no new viewers can join.
	stopped bool
}

// NewTrack takes ownership of a frame source. Run serves and closes it.
func NewTrack(source Source) *Track {
	// Expose keyframe requests only when the source can act on them.
	track := &Track{source: source}
	if source.CanRequestKeyframe() {
		track.requestKeyframe = source.RequestKeyframe
	}
	return track
}

// Subscribe adds an admitted control stream until ctx ends or release is called.
// The caller holds the control stream and its link for that lifetime.
func (t *Track) Subscribe(
	ctx context.Context,
	control link.MountedStream,
	subscription *Subscription,
	window int,
) (release func(), err error) {
	// Give the viewer an independent window on its existing link.
	write := WriteStreamGroups(OpenMountedGroupStream(control))
	sender, err := NewSender(ctx, subscription.GetSubscriptionId(), window, write, t.requestKeyframe)
	if err != nil {
		return nil, err
	}

	// Publish a new snapshot while a source is still available.
	t.mtx.Lock()
	if t.stopped {
		t.mtx.Unlock()
		// Close only cancels the sender's context.
		_ = sender.Close()
		return nil, io.EOF
	}
	t.senders = append(slices.Clone(t.senders), sender)
	t.mtx.Unlock()

	// Remove the sender on either explicit release or subscription shutdown.
	context.AfterFunc(sender.ctx, func() {
		t.mtx.Lock()
		defer t.mtx.Unlock()
		if i := slices.Index(t.senders, sender); i >= 0 {
			t.senders = slices.Delete(slices.Clone(t.senders), i, i+1)
		}
	})
	return sender.cancel, nil
}

// Run reads the source until it ends or ctx is canceled.
// Call Run once. On return it closes the source and all subscriptions.
func (t *Track) Run(ctx context.Context) (err error) {
	// Stop admitting viewers before shutting down the source and its senders.
	defer func() {
		// Detach the subscription snapshot before closing its writers.
		t.mtx.Lock()
		t.stopped = true
		senders := t.senders
		t.senders = nil
		t.mtx.Unlock()

		// Cancel the writers without holding the track lock.
		for _, sender := range senders {
			// Close only cancels the sender's context.
			_ = sender.Close()
		}

		// Release the source even when a frame read failed.
		if closeErr := t.source.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	for {
		// Read a picture once, regardless of the number of viewers.
		header, frame, err := t.source.ReadFrame(ctx)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		// Publish the same immutable frame to each independent sender.
		t.mtx.Lock()
		senders := t.senders
		t.mtx.Unlock()
		for _, sender := range senders {
			sender.Send(header, frame)
		}
	}
}
