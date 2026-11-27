package s4db

import (
	"context"
	"sync"
	"time"
)

// quietMark is how long a file stays quiet, with pending chunks waiting,
// before the scheduler flushes and appends a mark record so they can be
// freed.
const quietMark = time.Second

// memtableStall is the multiple of the budget at which background-marked
// commits wait for a flush.
const memtableStall = 2

// memtableCeiling is the multiple of the budget at which every commit waits
// for a flush.
const memtableCeiling = 8

// scheduler runs this handle's background jobs: it watches the published
// state for the flush trigger, runs the flusher, releases
// what the jobs made pending, and marks a quiet file so its pending chunks
// can be freed.
type scheduler struct {
	// releaseCursor resumes bounded free batches without rescanning their prefix.
	// mtx guards it with the other release state.
	releaseCursor int
	// marked is the published seq after the last mark step. A quiet file
	// marks again only once a record follows it. The loop alone uses it.
	marked uint64
	// releaseRemaining counts the entries left in a bounded release pass.
	// mtx guards releaseRemaining. A positive count schedules the next
	// batch immediately.
	releaseRemaining int
	// db is the database.
	db *DB
	// idle runs background reads and writes at background priority.
	idle *idleThread
	// fg tracks foreground commits for the quiet gate.
	fg quiet
	// cleaner evacuates value chunks.
	cleaner *cleaner
	// ctx and cancel end the jobs and their context, respectively.
	ctx    context.Context
	cancel context.CancelFunc
	// done is closed when the loop has stopped.
	done chan struct{}
	// mtx serializes the release step, which several jobs call.
	mtx sync.Mutex
}

// startScheduler publishes db's scheduler before starting its background loop.
func startScheduler(db *DB) {
	// Start the loop with its own context, cancelled by stop.
	ctx, cancel := context.WithCancel(context.Background())
	s := &scheduler{db: db, idle: newIdleThread(ctx), ctx: ctx, cancel: cancel, done: make(chan struct{})}
	s.cleaner = &cleaner{db: db}
	db.sched = s
	go s.run()
}

// stop ends the jobs and waits for the loop. A running flush finishes its
// current step first.
func (s *scheduler) stop() {
	s.cancel()
	<-s.done
}

// run waits for a trigger, runs the job it asks for, and sleeps until the
// state changes or the quiet timer fires.
func (s *scheduler) run() {
	defer close(s.done)
	quietAt := time.Now()
	for s.ctx.Err() == nil {
		// Read the state and the wait channel together.
		var st *state
		var wait <-chan struct{}
		s.db.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			st = s.db.cur.Load()
			wait = getWaitCh()
		})

		// Finish a known release pass before starting work that needs its space.
		// Every batch reacquires writer admission, so foreground requests still win.
		if s.releaseQueued() {
			if err := s.releaseStep(s.ctx); err == nil {
				quietAt = time.Now()
				continue
			}
		}

		// Flush when the memtable or the log since the last flush passed the
		// budget. The writer-owner lock excludes another process's scheduler.
		if st.flushDue(s.db.budget) && s.flush() {
			quietAt = time.Now()
			continue
		}

		// Merge when a level is over its target, in a quiet spell, or
		// at once when level 0 passed its stall. The gate goes first,
		// since the pick compares runs.
		if s.fg.wait() <= 0 || st.stalled() {
			if _, ok := (&mergeJob{db: s.db}).pick(st.m); ok && s.merge() {
				quietAt = time.Now()
				continue
			}
		}

		// Clean when the slack passed its bound, in a quiet spell unless
		// urgent.
		if urgent, ok := s.cleaner.due(st.m); ok && (urgent || s.fg.wait() <= 0) {
			if s.clean() {
				quietAt = time.Now()
				continue
			}
		}

		// Shrink a file whose tail is free, a release step at a time in
		// a quiet spell. Each job's release step also takes one.
		if _, shrinks := st.shrinkEnd(1); shrinks && s.fg.wait() <= 0 {
			if s.releaseStep(s.ctx) == nil {
				quietAt = time.Now()
				continue
			}
		}

		// Mark a quiet file whose pending chunks wait. A mark helps only the
		// chunks recorded since the last one, so with no record since, the
		// release step alone frees those whose pins have ended, without a
		// mark record and its two flushes every quiet second.
		if time.Since(quietAt) >= quietMark && st.hasPending() {
			if st.seq > s.marked {
				_ = s.markStep(s.ctx)
				s.marked = s.db.cur.Load().seq
			} else {
				_ = s.releaseStep(s.ctx)
			}
			quietAt = time.Now()
			continue
		}

		// Sleep until the state changes, the quiet spell opens, or the quiet
		// timer fires.
		wake := quietMark
		if w := s.fg.wait(); w > 0 && w < wake {
			wake = w
		}
		t := time.NewTimer(wake)
		select {
		case <-s.ctx.Done():
			t.Stop()
			return
		case <-wait:
			t.Stop()
			quietAt = time.Now()
		case <-t.C:
		}
	}
}

// flush runs one flush. Only the scheduler's loop runs flush jobs.
func (s *scheduler) flush() bool {
	return (&flushJob{db: s.db}).run(s.ctx) == nil
}

// merge runs one merge. The scheduler runs jobs serially.
func (s *scheduler) merge() bool {
	return (&mergeJob{db: s.db}).run(s.ctx) == nil
}

// clean runs one cleaner step on the scheduler's single loop.
func (s *scheduler) clean() bool {
	ran, err := s.cleaner.step(s.ctx)
	return ran && err == nil
}

// markStep flushes, appends a mark record carrying the mark that flush
// raised, and flushes again so the mark record is itself durable, then runs
// the release step.
func (s *scheduler) markStep(ctx context.Context) error {
	// Flush, so the mark covers every commit.
	db := s.db
	if err := db.flush.syncThrough(ctx, db.cur.Load().seq); err != nil {
		return err
	}

	// Append the mark record and make it durable.
	err := db.w.locked(ctx, func() error {
		return db.w.mark(ctx)
	})
	if err != nil {
		return err
	}
	if err := db.flush.syncThrough(ctx, db.cur.Load().seq); err != nil {
		return err
	}
	return s.releaseStep(ctx)
}

// waitMemtable blocks a commit while the memtable is past its ceiling, or
// past the stall point for a background-marked commit, until a flush brings
// it back.
func (db *DB) waitMemtable(ctx context.Context, background bool) error {
	// Wait for the state to come back within the limit.
	limit := memtableCeiling * db.budget
	if background {
		limit = memtableStall * db.budget
	}
	start := time.Now()
	err := db.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		if db.closed.Load() {
			return false, ErrClosed
		}
		st := db.cur.Load()
		if background && st.stalled() {
			return false, nil
		}
		return st.mem.bytes <= limit, nil
	})
	if d := time.Since(start); d > time.Millisecond {
		db.recordWait(d)
	}
	return err
}
