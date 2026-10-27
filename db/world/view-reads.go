package world

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// viewReads collects the reads and next deadline of one view evaluation.
// All work using its context must finish before the evaluation returns.
type viewReads struct {
	// reads unions the reads of every transaction opened during the evaluation.
	reads *ReadSet
	// mtx guards shared and wake.
	mtx sync.Mutex
	// shared are the finished read sets added by AddViewReads.
	shared []*ReadSet
	// wake is the earliest requested rerun time, or zero.
	wake time.Time
}

// viewReadsKey identifies the evaluation carried by a context.
type viewReadsKey struct{}

// getViewReads returns the evaluation carried by ctx, or nil outside a view.
func getViewReads(ctx context.Context) *viewReads {
	reads, _ := ctx.Value(viewReadsKey{}).(*viewReads)
	return reads
}

// RerunAt asks the running WatchView or SendViews evaluation to run again at
// deadline even without a World change. The earliest nonzero deadline wins.
// It does nothing outside an evaluation. Call it before the evaluation returns.
func RerunAt(ctx context.Context, deadline time.Time) {
	// Ignore absent evaluations and absent deadlines.
	reads := getViewReads(ctx)
	if reads == nil || deadline.IsZero() {
		return
	}

	// Keep the earliest deadline across concurrent reads.
	reads.mtx.Lock()
	if reads.wake.IsZero() || deadline.Before(reads.wake) {
		reads.wake = deadline
	}
	reads.mtx.Unlock()
}

// AddViewReads adds a finished read set to the running WatchView or SendViews
// evaluation, so the view reruns when a change touches set, and asks for a
// rerun at rerunAt like RerunAt. Use it for a value computed once on its own
// tracked transaction and shared by many views. The caller must not add reads to
// set afterward. It does nothing outside an evaluation. Call it before the
// evaluation returns.
func AddViewReads(ctx context.Context, set *ReadSet, rerunAt time.Time) {
	// Ignore absent evaluations.
	reads := getViewReads(ctx)
	if reads == nil {
		return
	}

	// Join the set once.
	if set != nil {
		reads.mtx.Lock()
		if !slices.Contains(reads.shared, set) {
			reads.shared = append(reads.shared, set)
		}
		reads.mtx.Unlock()
	}
	RerunAt(ctx, rerunAt)
}

// WithViewReads starts a child evaluation that records into set, for a value
// computed once and shared by many views. Transactions opened through a
// ReadTrackingEngine with the returned context, and AddViewReads and RerunAt
// calls made with it, record into set and the child's deadline instead of the
// running evaluation; nothing reaches the parent until the caller passes the
// finished set to AddViewReads. finish joins the sets the child added to set
// by reference and returns its earliest rerun time, or zero; call it once,
// after every read made with the context. It works outside a view too.
func WithViewReads(ctx context.Context, set *ReadSet) (child context.Context, finish func() time.Time) {
	reads := &viewReads{reads: set}
	return context.WithValue(ctx, viewReadsKey{}, reads), func() time.Time {
		for _, shared := range reads.getShared() {
			set.join(shared)
		}
		return reads.getDeadline()
	}
}

// getShared returns the finished read sets added to the evaluation.
func (r *viewReads) getShared() []*ReadSet {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return slices.Clone(r.shared)
}

// getDeadline returns the earliest rerun time requested by the evaluation.
func (r *viewReads) getDeadline() time.Time {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.wake
}

// wait ends when the evaluation's reads change or its deadline arrives.
func (r *viewReads) wait(ctx context.Context, source WorldWaitSeqno, seqno uint64) error {
	// A deadline belongs only to the wait, not the evaluation's reads.
	wait := ctx
	if deadline := r.getDeadline(); !deadline.IsZero() {
		var cancel context.CancelFunc
		wait, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}

	// Distinguish our rerun deadline from caller cancellation and watch errors.
	_, err := r.reads.waitChange(wait, source, seqno, r.getShared())
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) && wait.Err() == context.DeadlineExceeded {
		return nil
	}
	return err
}
