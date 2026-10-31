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
// state for the flush trigger, runs the flusher under lockFlush, releases
// what the jobs made pending, and marks a quiet file so its pending chunks
// can be freed.
type scheduler struct {
	// db is the database.
	db *DB
	// idle runs background reads and writes at background priority.
	idle *idleThread
	// fg tracks foreground commits for the quiet gate.
	fg quiet
	// cleaner evacuates value chunks.
	cleaner *cleaner
	// ctx ends the jobs; cancel ends it.
	ctx    context.Context
	cancel context.CancelFunc
	// done is closed when the loop has stopped.
	done chan struct{}
	// mtx serializes the release step, which several jobs call.
	mtx sync.Mutex
}

// newScheduler starts the scheduler of db.
func newScheduler(db *DB) *scheduler {
	// Start the loop with its own context, cancelled by stop.
	ctx, cancel := context.WithCancel(context.Background())
	s := &scheduler{db: db, idle: newIdleThread(ctx), ctx: ctx, cancel: cancel, done: make(chan struct{})}
	s.cleaner = &cleaner{db: db}
	go s.run()
	return s
}

// stop ends the jobs and waits for the loop; a running flush finishes its
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

		// Flush when the memtable or the log since the last flush passed the
		// budget, and merge when a level is over its target.
		if st.mem.len() != 0 && (st.mem.bytes > s.db.budget || st.logBytes > 4*s.db.budget) {
			s.flush()
			quietAt = time.Now()
			continue
		}
		// Merge in a quiet spell, or at once when level 0 passed its stall.
		if _, ok := (&mergeJob{db: s.db}).pick(st.m); ok && (s.fg.wait() <= 0 || st.m.level0() > level0Stall) {
			if s.merge() {
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

		// Shrink a file whose tail is free, a release step at a time in a
		// quiet spell; each job's release step also takes one.
		if s.fg.wait() <= 0 && st.m.table.trimEnd(1) != st.m.table.end() {
			if s.releaseStep(s.ctx) == nil {
				quietAt = time.Now()
				continue
			}
		}

		// Mark a quiet file whose pending chunks wait.
		if time.Since(quietAt) >= quietMark && s.hasPending(st) {
			_ = s.markStep(s.ctx)
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

// flush runs one flush when no other process is flushing.
func (s *scheduler) flush() {
	// Take the flush lock without waiting.
	ok, err := s.db.s.lock(lockFlush, false)
	if err != nil || !ok {
		return
	}
	defer func() { _ = s.db.s.unlock(lockFlush) }()

	// Run the job; a failure leaves the state for the next trigger.
	_ = (&flushJob{db: s.db}).run(s.ctx)
}

// merge runs one merge when no other process is merging, reporting whether
// it ran.
func (s *scheduler) merge() bool {
	// Take the merge lock without waiting.
	ok, err := s.db.s.lock(lockMerge, false)
	if err != nil || !ok {
		return false
	}
	defer func() { _ = s.db.s.unlock(lockMerge) }()
	return (&mergeJob{db: s.db}).run(s.ctx) == nil
}

// clean runs one cleaner step when no other process is cleaning, reporting
// whether it cleaned a victim.
func (s *scheduler) clean() bool {
	// Take the clean lock without waiting.
	ok, err := s.db.s.lock(lockClean, false)
	if err != nil || !ok {
		return false
	}
	defer func() { _ = s.db.s.unlock(lockClean) }()
	ran, err := s.cleaner.step(s.ctx)
	return ran && err == nil
}

// hasPending reports whether the table holds pending or reserved chunks.
func (s *scheduler) hasPending(st *state) bool {
	for _, e := range st.m.table.entries {
		if e.kind == chunkPending || e.kind == chunkReserved {
			return true
		}
	}
	return false
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
		_, err := db.w.appendRecord(ctx, &record{kind: kindCommit})
		return err
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
		if background && st.m.level0() > level0Stall {
			return false, nil
		}
		return st.mem.bytes <= limit, nil
	})
	if d := time.Since(start); d > time.Millisecond {
		db.recordWait(d)
	}
	return err
}
