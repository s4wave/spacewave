package world

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// ChangeFilter reports whether a ChangeSet concerns a watcher. A nil filter
// accepts every change, and an unknown ChangeSet passes every filter.
type ChangeFilter func(changes *ChangeSet) bool

// Accepts reports whether the filter lets changes through.
func (f ChangeFilter) Accepts(changes *ChangeSet) bool {
	return f == nil || changes.Unknown || f(changes)
}

// ChangeWatcher is an Engine that reports what changed between seqnos.
type ChangeWatcher interface {
	// WatchChanges calls cb with each later seqno whose changes touches
	// accepts, starting after afterSeqno, and the accepted changes since the
	// previous call. Changes touches rejects never wake the watcher. Calls
	// coalesce while cb runs. Returns when ctx is canceled or cb returns an
	// error.
	WatchChanges(ctx context.Context, afterSeqno uint64, touches ChangeFilter, cb func(seqno uint64, changes *ChangeSet) error) error
}

// IndexedChangeWatcher is a ChangeWatcher that indexes its watchers by the
// names in their ChangeInterests, so a change tests and wakes only the
// watchers it names. An Engine wrapper that forwards WatchChanges forwards
// WatchIndexedChanges too, or its watchers fall back to WatchChanges.
type IndexedChangeWatcher interface {
	ChangeWatcher
	// WatchIndexedChanges is WatchChanges for the changes interest accepts. A
	// change that names none of the interest's keys or quad fields never tests
	// or wakes the watcher.
	WatchIndexedChanges(ctx context.Context, afterSeqno uint64, interest *ChangeInterest, cb func(seqno uint64, changes *ChangeSet) error) error
}

// WatchChanges calls cb with each seqno of w after afterSeqno and the changes
// since the previous call. w is an Engine or a WorldState; one that is not a
// ChangeWatcher reports every advance as an unknown ChangeSet.
// Returns when ctx is canceled or cb returns an error.
func WatchChanges(
	ctx context.Context,
	w WorldWaitSeqno,
	afterSeqno uint64,
	cb func(seqno uint64, changes *ChangeSet) error,
) error {
	return WatchTouchingChanges(ctx, w, afterSeqno, nil, cb)
}

// WatchTouchingChanges is WatchChanges for the changes touches accepts. A
// World with a change feed tests each change once on the feed and wakes only
// the watchers it concerns.
func WatchTouchingChanges(
	ctx context.Context,
	w WorldWaitSeqno,
	afterSeqno uint64,
	touches ChangeFilter,
	cb func(seqno uint64, changes *ChangeSet) error,
) error {
	// Prefer the World's own change feed.
	if watcher, ok := w.(ChangeWatcher); ok {
		return watcher.WatchChanges(ctx, afterSeqno, touches, cb)
	}

	// Report each seqno advance as unknown, which every filter accepts.
	for {
		seqno, err := w.WaitSeqno(ctx, afterSeqno+1)
		if err != nil {
			return err
		}
		if err := cb(seqno, NewUnknownChangeSet()); err != nil {
			return err
		}
		afterSeqno = seqno
	}
}

// WatchInterestedChanges is WatchTouchingChanges for the changes interest
// accepts. A World whose change feed indexes its watchers tests interest only
// on the changes it names; any other World tests it as a ChangeFilter.
func WatchInterestedChanges(
	ctx context.Context,
	w WorldWaitSeqno,
	afterSeqno uint64,
	interest *ChangeInterest,
	cb func(seqno uint64, changes *ChangeSet) error,
) error {
	if watcher, ok := w.(IndexedChangeWatcher); ok {
		return watcher.WatchIndexedChanges(ctx, afterSeqno, interest, cb)
	}
	return WatchTouchingChanges(ctx, w, afterSeqno, interest.Accepts, cb)
}

// WatchViewOptions tunes WatchViewWith. The zero value is WatchView's behavior.
type WatchViewOptions struct {
	// AfterSeqno is a seqno the caller already holds the view at. When it is
	// above zero, the first evaluation waits for the World to pass it.
	AfterSeqno uint64
	// MinInterval is the least time between the starts of two evaluations. A
	// burst of touching commits reruns the view once per interval, and the rerun
	// reads the newest head. Zero reruns as soon as a change or deadline arrives.
	MinInterval time.Duration
}

// WatchView runs view immediately and again when a change touches its reads or
// its earliest RerunAt deadline arrives. It is WatchViewWith with zero options.
func WatchView(ctx context.Context, source WorldWaitSeqno, view func(ctx context.Context, ws WorldState) error) error {
	return WatchViewWith(ctx, source, WatchViewOptions{}, view)
}

// WatchViewWith runs view like WatchView, tuned by opts. source must be an
// Engine or WorldState; an Engine supplies a live, read-only WorldState. Every
// evaluation has a fresh read set carried by its context. Transactions opened
// through a ReadTrackingEngine with that context join the same set, including
// transactions opened by clients captured by view, and AddViewReads joins a
// finished set. All transactions must read source's World, and all evaluation
// work must finish before view returns. Unknown changes can rerun an unchanged
// projection; use SendViews to suppress equal results.
// Returns when ctx ends or view or the watch fails.
func WatchViewWith(
	ctx context.Context,
	source WorldWaitSeqno,
	opts WatchViewOptions,
	view func(ctx context.Context, ws WorldState) error,
) error {
	// Preserve an existing state's access policy, or build live engine reads.
	ws, ok := source.(WorldState)
	if !ok {
		engine, ok := source.(Engine)
		if !ok {
			return errors.New("view source must be an Engine or WorldState")
		}
		ws = NewEngineWorldState(engine, false)
	}

	// A caller that already holds a seqno needs only later changes.
	if opts.AfterSeqno > 0 {
		if _, err := source.WaitSeqno(ctx, opts.AfterSeqno+1); err != nil {
			return err
		}
	}

	// Keep one change feed subscription across the waits, so each rerun's
	// wait moves it by the reads that changed instead of renaming them all.
	slot := &WatchSlot{}
	defer slot.Close()
	waitCtx := WithWatchSlot(ctx, slot)

	// Record each evaluation's starting seqno before reading its dependencies.
	for {
		// Do not start another evaluation after cancellation.
		if err := ctx.Err(); err != nil {
			return err
		}

		// Run the view against the head, recording its reads.
		started := time.Now()
		seqno, err := source.GetSeqno(ctx)
		if err != nil {
			return err
		}
		reads := &viewReads{reads: NewReadSet()}
		if err := view(context.WithValue(ctx, viewReadsKey{}, reads), reads.reads.Track(ws)); err != nil {
			return err
		}

		// Wait for a relevant change, bounded by the evaluation's deadline.
		if err := reads.wait(waitCtx, source, seqno); err != nil {
			return err
		}

		// Space reruns, so the next evaluation reads the newest head.
		if err := sleepUntil(ctx, started.Add(opts.MinInterval)); err != nil {
			return err
		}
	}
}

// sleepUntil waits for t, returning early with ctx's error if ctx ends first.
func sleepUntil(ctx context.Context, t time.Time) error {
	// Skip the timer when t has passed.
	d := time.Until(t)
	if d <= 0 {
		return nil
	}

	// Wait for the timer or cancellation.
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
