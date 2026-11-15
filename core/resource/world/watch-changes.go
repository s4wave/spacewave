package resource_world

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// ErrInterestVersion is returned for a first interest whose version is not 1,
// an interest delta whose version is not one more than the previous delta's,
// or a delta on a stream opened without an interest. The client reopens the
// stream with its whole interest.
var ErrInterestVersion = errors.New("interest delta out of order")

// WatchChanges streams each later World revision with the changes since the
// previous message. A stream opened without an interest sends every change
// and reads no more requests. A stream opened with one sends the changes it
// names, applies each later delta at the last seqno it sent, and acknowledges
// it there. The end of the request side ends only the deltas.
func (r *EngineResource) WatchChanges(stream s4wave_world.SRPCEngineResourceService_WatchChangesStream) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	switch first.GetInterest().GetVersion() {
	case 0:
		if first.GetInterest() != nil {
			return ErrInterestVersion
		}
		return r.watchAllChanges(stream, first.GetAfterSeqno())
	case 1:
		return r.watchNamedChanges(stream.Context(), stream, first)
	default:
		return ErrInterestVersion
	}
}

// watchAllChanges serves a stream opened without an interest: every change,
// through the same call the change feed serves any unfiltered watcher. A
// later request fails the stream with ErrInterestVersion, and a failed read
// with its own error, so none queue unread.
func (r *EngineResource) watchAllChanges(
	stream s4wave_world.SRPCEngineResourceService_WatchChangesStream,
	afterSeqno uint64,
) error {
	// End the watch on any later request.
	ctx, cancel := context.WithCancelCause(stream.Context())
	defer cancel(nil)
	go func() {
		_, err := stream.Recv()
		switch {
		case err == nil:
			cancel(ErrInterestVersion)
		case !errors.Is(err, io.EOF):
			cancel(err)
		}
	}()
	err := world.WatchChanges(ctx, r.engine, afterSeqno, func(seqno uint64, changes *world.ChangeSet) error {
		return stream.Send(s4wave_world.NewChangeResponse(seqno, changes))
	})
	if cause := context.Cause(ctx); cause != nil && stream.Context().Err() == nil {
		return cause
	}
	return err
}

// watchNamedChanges serves a stream opened with an interest. One goroutine
// reads deltas into the interest and never sends; this one sends, and runs
// the change watch under the interest it last took. A delta ends that watch,
// which returns after its last Send, at the seqno L it sent or covered
// through; the next watch starts after
// L under the new interest, behind an ack carrying L, and catches up from L
// through the same subscriber, kept in a WatchSlot, so no change after L is
// lost and none at or before L is sent again.
func (r *EngineResource) watchNamedChanges(
	ctx context.Context,
	stream s4wave_world.SRPCEngineResourceService_WatchChangesStream,
	first *s4wave_world.WatchChangesRequest,
) error {
	// Start from the first request's interest and read the deltas after it.
	in := newInterestInbox()
	if err := in.names.apply(first.GetInterest()); err != nil {
		return err
	}
	in.version = 1
	go in.read(stream)

	// Keep one change watch subscriber across the interest's changes.
	slot := new(world.WatchSlot)
	defer slot.Close()
	ctx = world.WithWatchSlot(ctx, slot)

	// Send each change and advance past it. A watch that ends also advances
	// past the changes its interest skipped, through the seqno it covered.
	afterSeqno, acked := first.GetAfterSeqno(), in.version
	send := func(seqno uint64, changes *world.ChangeSet) error {
		if err := stream.Send(s4wave_world.NewChangeResponse(seqno, changes)); err != nil {
			return err
		}
		afterSeqno = seqno
		return nil
	}
	for {
		// Take the interest; a delta read from here on ends the watch.
		watchCtx, interest, version, err := in.take(ctx)
		if err != nil {
			return err
		}

		// Acknowledge the deltas taken: every change after afterSeqno is
		// filtered under them.
		if version != acked {
			if err := stream.Send(s4wave_world.NewInterestAckResponse(version, afterSeqno)); err != nil {
				return err
			}
			acked = version
		}

		// Watch until a delta, a failure or the end of the stream. Only a
		// delta ends the watch while the stream lives, by canceling watchCtx.
		err = world.WatchInterestedChanges(watchCtx, r.engine, afterSeqno, interest, send)
		ended := watchCtx.Err() != nil
		afterSeqno = max(afterSeqno, slot.Covered())
		if ferr := in.release(); ferr != nil {
			return ferr
		}
		if ctx.Err() != nil || !ended {
			return err
		}
	}
}

// interestInbox holds a stream's interest as the deltas read so far leave it.
// The reader folds each delta in, so it holds at most the interest's own
// names, however many deltas arrive while the sender is blocked.
type interestInbox struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// names is the interest after every delta read.
	names interestNames
	// version is the version of the last delta read.
	version uint64
	// err ends the stream: a bad delta, or a failed read.
	err error
	// cancel ends the running watch, or is nil between watches.
	cancel context.CancelFunc
	// taken is the version the running watch took.
	taken uint64
}

// newInterestInbox returns an inbox holding an empty interest.
func newInterestInbox() *interestInbox {
	return &interestInbox{names: newInterestNames()}
}

// read applies each delta on stream until the request side ends. The end of
// the request side ends only the deltas; a failed read or a bad delta ends
// the stream.
func (in *interestInbox) read(stream s4wave_world.SRPCEngineResourceService_WatchChangesStream) {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		in.mtx.Lock()
		if err == nil {
			err = in.applyLocked(req.GetInterest())
		}
		if err != nil && in.err == nil {
			in.err = err
		}
		if in.cancel != nil && (err != nil || in.version != in.taken) {
			in.cancel()
		}
		in.mtx.Unlock()
		if err != nil {
			return
		}
	}
}

// applyLocked applies one delta. The caller holds mtx.
func (in *interestInbox) applyLocked(delta *s4wave_world.ChangeInterestDelta) error {
	if delta == nil || delta.GetVersion() != in.version+1 {
		return ErrInterestVersion
	}
	if err := in.names.apply(delta); err != nil {
		return err
	}
	in.version = delta.GetVersion()
	return nil
}

// take returns the interest for the next watch, with its version and a
// context a later delta cancels.
func (in *interestInbox) take(ctx context.Context) (context.Context, *world.ChangeInterest, uint64, error) {
	// Fail on a bad delta before building anything.
	in.mtx.Lock()
	defer in.mtx.Unlock()
	if in.err != nil {
		return nil, nil, 0, in.err
	}

	// Build the interest and the context a later delta cancels.
	interest, err := in.names.interest()
	if err != nil {
		return nil, nil, 0, err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	in.cancel, in.taken = cancel, in.version
	return watchCtx, interest, in.version, nil
}

// release ends the running watch's context and returns the error that ends
// the stream, if any.
func (in *interestInbox) release() error {
	in.mtx.Lock()
	defer in.mtx.Unlock()
	if in.cancel != nil {
		in.cancel()
		in.cancel = nil
	}
	return in.err
}

// patternKey is a graph pattern as a delta spells it.
type patternKey struct {
	subject, predicate, obj, label string
}

// interestNames is a named interest: object keys and graph patterns.
type interestNames struct {
	// keys are the object keys.
	keys map[string]struct{}
	// patterns are the graph patterns.
	patterns map[patternKey]struct{}
}

// newInterestNames returns an empty interest.
func newInterestNames() interestNames {
	return interestNames{keys: make(map[string]struct{}), patterns: make(map[patternKey]struct{})}
}

// apply adds and removes the names in delta. A pattern naming no field fails
// it before any change.
func (n interestNames) apply(delta *s4wave_world.ChangeInterestDelta) error {
	// Reject a pattern naming no field before changing anything.
	for _, q := range delta.GetAddPatterns() {
		if q.GetSubject() == "" && q.GetPredicate() == "" && q.GetObj() == "" && q.GetLabel() == "" {
			return world.ErrGraphPatternNamesNoField
		}
	}

	// Remove, then add, so a name both removed and added stays.
	for _, key := range delta.GetRemoveKeys() {
		delete(n.keys, key)
	}
	for _, q := range delta.GetRemovePatterns() {
		delete(n.patterns, newPatternKey(q))
	}
	for _, key := range delta.GetAddKeys() {
		n.keys[key] = struct{}{}
	}
	for _, q := range delta.GetAddPatterns() {
		n.patterns[newPatternKey(q)] = struct{}{}
	}
	return nil
}

// interest builds the change interest the names describe.
func (n interestNames) interest() (*world.ChangeInterest, error) {
	// Spell the names as lists.
	keys := make([]string, 0, len(n.keys))
	for key := range n.keys {
		keys = append(keys, key)
	}
	patterns := make([]world.GraphQuad, 0, len(n.patterns))
	for p := range n.patterns {
		patterns = append(patterns, world.NewGraphQuad(p.subject, p.predicate, p.obj, p.label))
	}
	return world.NewNamedInterest(keys, patterns)
}

// newPatternKey returns the key of a delta's pattern.
func newPatternKey(q *quad.Quad) patternKey {
	return patternKey{q.GetSubject(), q.GetPredicate(), q.GetObj(), q.GetLabel()}
}
