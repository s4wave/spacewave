package common

import (
	"context"
	"database/sql"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// walFlow holds back a writer whose commits outrun the checkpointer, after
// each such commit has landed, until the checkpointer has copied it.
//
// SQLite resets the log only for a writer that begins on a fully copied log,
// and fillWALGap opens that gap under the writer turn only when a pass leaves
// at most walGapFrames frames to copy within walTurnHold. A writer whose
// commits each add more than that, back to back, never leaves such a gap, so
// without the flow its log grows without bound. No hold of the turn bounded
// by walTurnHold can copy such a commit, so the flow charges the writer that
// made it instead: once a commit leaves the log at walHoldMultiple times its
// bound and added more than walGapFrames frames, the writer waits for the
// checkpointer outside the turn, and every other writer and reader goes on.
//
// The wait begins after the commit has landed and released the turn, so it
// holds no lock and fails no commit: a cancelled context ends it early, and
// the commit stands. It ends once the checkpointer goes idle after a pass
// that began after the commit. The checkpointer goes idle only with the log
// under walHoldMultiple times its bound, which past it means SQLite reset
// the log; once a pass copied the whole log, which the next writer to begin
// resets, and that writer may be the one waiting; once a reader pins it; or
// once a pass fails, which no wait can mend. A log a reader pins cannot reset, so while the last pass found
// it pinned no commit waits, and the finding ends every wait at once. The
// wait's length is the copy of the frames behind the commit, at most the
// commit's own frames plus those of the commits in flight beside it, over
// the copy rate: about 24 MiB/s on forge, so 2.2 s for a 54 MB commit.
//
// Invariant: a commit of at most walGapFrames frames never waits here, so a
// writer of small commits keeps fillWALGap's bound of one walTurnHold plus
// the gap's flush behind the checkpointer.
//
// Several writers: past walHoldMultiple times the bound the checkpointer
// goes idle only once a pass copied the whole log, and each pass holds the writers
// back from its start, letting each waiting writer one commit per
// walTurnHold. A large writer that commits there waits, so once every large
// writer waits, only small writers' commits land between passes; the first
// caught-up pass after them finds at most walGapFrames new frames, takes the
// turn ahead of the writers, since the log is past walGapQueueMultiple times
// its bound, and closes the gap; the checkpointer goes idle, and the next
// writer, small or released from its wait, resets the log. So
// unless a reader pins it, the log stays under walHoldMultiple times its
// bound plus one commit of each large writer in flight at once and one
// commit of each small writer per walTurnHold while the passes copy them.
type walFlow struct {
	// size returns the log's size on this host, or zero when unknown.
	size func() int64
	// wake wakes the checkpointer.
	wake func()
	// hold is walHoldMultiple times the store's log bound.
	hold int64
	// stats counts the waits.
	stats *storeStats

	// last is the log's size after the last commit, and frameBytes the size
	// of one log frame, zero until read; both only under the writer turn.
	last, frameBytes int64
	// commits counts commits, in the order they wake the checkpointer.
	commits atomic.Uint64
	// round is the commit count when the checkpointer's last pass began,
	// which only the checkpointer moves and reads.
	round uint64

	// mtx guards the fields below.
	mtx sync.Mutex
	// settled is round when the checkpointer last went idle.
	settled uint64
	// pinned reports that the checkpointer's last pass found a reader
	// pinning the log.
	pinned bool
	// idle is closed and replaced each time the checkpointer goes idle.
	idle chan struct{}
}

// newWALFlow returns the flow of a store whose log is bound at bound bytes.
func newWALFlow(size func() int64, wake func(), bound int64, stats *storeStats) *walFlow {
	return &walFlow{size: size, wake: wake, hold: walHoldMultiple * bound, stats: stats, idle: make(chan struct{})}
}

// outran reports whether the commit that conn just made left the log at
// walHoldMultiple times its bound with more than walGapFrames frames added.
// It runs under the writer turn, after COMMIT, and costs one stat of the log
// and, on a store's first commit, one read of its page size.
func (f *walFlow) outran(conn *sql.Conn) bool {
	// Measure the commit against the log after the last one. A commit
	// that SQLite restarted the log for wrote from its first frame, and
	// SQLite then trimmed the file to the larger of its limit and that
	// commit's frames, so a file that did not grow is all its own.
	size := f.size()
	added := size - f.last
	if added <= 0 {
		added = size
	}
	f.last = size
	f.stats.addLog(size)
	if size < f.hold {
		return false
	}

	// Count the added bytes in frames of the store's page.
	if f.frameBytes == 0 {
		var page int64
		if err := conn.QueryRowContext(context.Background(), "PRAGMA page_size").Scan(&page); err != nil || page <= 0 {
			return false
		}
		f.frameBytes = page + walFrameHeaderBytes
	}
	return added > walGapFrames*f.frameBytes
}

// walFrameHeaderBytes is the header SQLite writes before each page in the log.
const walFrameHeaderBytes = 24

// committed counts a commit and wakes the checkpointer, and returns the
// commit's place for wait.
func (f *walFlow) committed() uint64 {
	seq := f.commits.Add(1)
	f.wake()
	return seq
}

// wait waits until the checkpointer goes idle after a pass that began after
// commit seq, or ctx ends.
func (f *walFlow) wait(ctx context.Context, seq uint64) {
	start := time.Now()
	defer func() { f.stats.addFlowWait(time.Since(start)) }()
	for {
		f.mtx.Lock()
		settled, pinned, idle := f.settled, f.pinned, f.idle
		f.mtx.Unlock()
		if settled >= seq || pinned {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-idle:
		}
	}
}

// pass marks a checkpointer pass beginning; it copies every commit counted
// so far.
func (f *walFlow) pass() {
	f.round = f.commits.Load()
}

// settle marks the checkpointer idle until the next commit, ending the wait
// of every commit its last pass copied, and of every commit while pinned
// reports that a reader pins the log.
func (f *walFlow) settle(pinned bool) {
	f.publish(f.round, pinned)
}

// stop ends every wait for good once the checkpointer has returned.
func (f *walFlow) stop() {
	f.publish(math.MaxUint64, true)
}

// publish sets settled and pinned and wakes the waiters.
func (f *walFlow) publish(round uint64, pinned bool) {
	// Swap in a fresh idle channel and close the old one.
	f.mtx.Lock()
	f.settled, f.pinned = round, pinned
	close(f.idle)
	f.idle = make(chan struct{})
	f.mtx.Unlock()
}
