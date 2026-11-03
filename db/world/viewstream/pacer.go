package viewstream

import (
	"context"
	"sync"
	"time"

	"github.com/s4wave/spacewave/db/world"
)

// Pacer is the source one view stream watches. It spaces the stream's
// runs interval apart in place of the world library's MinInterval, which
// cannot tell one change from another, and lets a change through at once
// when the stream's actor wrote since the run it follows.
type Pacer struct {
	world.Engine
	// owners records each actor's writes; nil for a replica's Store.
	owners *Writes
	// actor is the application actor ID the stream acts for, or zero.
	actor uint64
	// pokes lets a change through at once when poked; nil for a stream
	// without them.
	pokes *Pokes
	// interval is the least time between the starts of two runs.
	interval time.Duration

	// mtx guards the fields below. The library runs the view and the change
	// watch one after the other, on different goroutines.
	mtx sync.Mutex
	// last is when the latest run started, or zero before the first.
	last time.Time
	// seqno is the revision the latest run read, whose later changes the
	// stream waits for.
	seqno uint64
}

// Started paces a run that a deadline woke, which skips WatchChanges, and
// marks the start of the run.
func (p *Pacer) Started(ctx context.Context) error {
	// Wait for this run's turn, then clear the poke it answers.
	if err := p.pace(ctx); err != nil {
		return err
	}
	p.mtx.Lock()
	p.last = time.Now()
	p.mtx.Unlock()
	if p.pokes != nil {
		p.pokes.Clear()
	}
	return nil
}

// WatchChanges holds each change that touches the stream until the interval
// since its latest run passes, or until the actor writes.
func (p *Pacer) WatchChanges(ctx context.Context, afterSeqno uint64, touches world.ChangeFilter, cb func(uint64, *world.ChangeSet) error) error {
	p.watched(afterSeqno)
	return world.WatchTouchingChanges(ctx, p.Engine, afterSeqno, touches, p.paced(ctx, cb))
}

// WatchIndexedChanges is WatchChanges over the World's indexed change feed,
// so a change wakes only the views whose reads it names.
func (p *Pacer) WatchIndexedChanges(ctx context.Context, afterSeqno uint64, interest *world.ChangeInterest, cb func(uint64, *world.ChangeSet) error) error {
	p.watched(afterSeqno)
	return world.WatchInterestedChanges(ctx, p.Engine, afterSeqno, interest, p.paced(ctx, cb))
}

// watched records afterSeqno, the revision the run read, which the library
// waits after.
func (p *Pacer) watched(afterSeqno uint64) {
	p.mtx.Lock()
	p.seqno = afterSeqno
	p.mtx.Unlock()
}

// paced passes each change on to cb once paced.
func (p *Pacer) paced(ctx context.Context, cb func(uint64, *world.ChangeSet) error) func(uint64, *world.ChangeSet) error {
	return func(seqno uint64, changes *world.ChangeSet) error {
		if err := p.pace(ctx); err != nil {
			return err
		}
		return cb(seqno, changes)
	}
}

// pace waits until interval after the latest run's start, returning at once
// when the actor wrote after the revision that run read or a poke came, and
// rechecking at the actor's next write.
func (p *Pacer) pace(ctx context.Context) error {
	for {
		// Read the latest run's start and revision.
		p.mtx.Lock()
		due, seqno := p.last.Add(p.interval), p.seqno
		p.mtx.Unlock()
		wait := time.Until(due)
		if wait <= 0 {
			return nil
		}

		// The actor's own write passes at once.
		var wake <-chan struct{}
		if p.actor != 0 && p.owners != nil {
			var own bool
			if own, wake = p.owners.Since(p.actor, seqno); own {
				return nil
			}
		}

		// A poke passes at once too.
		var poke <-chan struct{}
		if p.pokes != nil {
			var poked bool
			if poked, poke = p.pokes.Pending(); poked {
				return nil
			}
		}

		// Wait out the interval, or recheck at the actor's next write or
		// a poke.
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			return nil
		case <-wake:
			timer.Stop()
		case <-poke:
			timer.Stop()
		}
	}
}

// NewPacer spaces a view's reruns while allowing its actor's own writes and
// explicit pokes through immediately. The caller still owns first-read admission.
func NewPacer(engine world.Engine, writes *Writes, actor uint64, pokes *Pokes, interval time.Duration) *Pacer {
	return &Pacer{Engine: engine, owners: writes, actor: actor, pokes: pokes, interval: interval}
}

// _ is a type assertion
var _ world.IndexedChangeWatcher = (*Pacer)(nil)
