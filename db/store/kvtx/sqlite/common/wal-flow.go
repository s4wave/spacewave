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
// no more frames than the last pass measured it copies within
// walFlowWaitMax. A writer whose commits each add more than that, back to
// back, never leaves such a gap, so without the flow its log grows without
// bound. No hold of the turn bounded by walFlowWaitMax can copy such a
// commit, so the flow charges the writer that made it instead: once a commit
// leaves the log at walHoldMultiple times its bound and added more than
// walGapFrames frames, the writer waits for the checkpointer outside the
// turn, and every other writer and reader goes on.
//
// The wait begins after the commit has landed and released the turn, so it
// holds no lock and fails no commit: a cancelled context ends it early, and
// the commit stands. It ends once the checkpointer goes idle after a pass
// that began after the commit. The checkpointer goes idle only in these cases:
// The log is under walHoldMultiple times its bound. If the log exceeded that
// bound, going idle means SQLite reset the log.
// A pass copied the whole log, which the next writer to begin resets, and
// that writer may be the one waiting.
// A reader pins the log.
// A pass fails, and no wait can mend a failed pass.
// A reader the checkpointer does not judge a pin cuts a pass short.
// Another checkpointer on the same log makes a pass busy.
// In the last two cases the commit's frames may be left uncopied, and later
// passes copy them. A log a reader pins cannot reset, so
// while the last pass found it pinned no commit waits, and the finding ends
// every wait at once. The wait's length is the copy of the frames behind the
// commit, at most the commit's own frames plus those of the commits in
// flight beside it, over the copy rate: about 24 MiB/s on forge, so 2.2 s
// for a 54 MB commit.
//
// The wait also ends once a pass that began after the commit copied the
// whole log as it began, and the wait has lasted walFlowWaitMax: by then the
// commit is copied, and holding the commit on serves only to let the gap
// close. Small writers that together add more than walGapFrames frames per
// pass never let the gap close. So the wait is at most the copy of the
// commit's frames or walFlowWaitMax, whichever is longer. On a disk that cannot
// copy the gap within walFlowWaitMax the checkpointer never goes idle, and this
// deadline is what ends the wait.
//
// Invariant: a commit of at most walGapFrames frames never waits here. It
// waits only behind the checkpointer's hold of the writer turn, at most one
// per commit: a back-pressure slice of walTurnHold, or the gap pass of at
// most walGapHold (walFlowWaitMax) and the flush of the pages it copied.
//
// Several writers: past walHoldMultiple times the bound the checkpointer
// goes idle only once a pass copied the whole log, and each pass holds the
// writers back from its start, letting each waiting writer one commit per
// walTurnHold. A large writer that commits there waits, so once every large
// writer waits, only small writers' commits land between passes. The first
// caught-up pass after them finds new frames it can copy within
// walFlowWaitMax, takes the turn ahead of the writers, since the log is past
// walGapQueueMultiple times its bound, and closes the gap. The checkpointer
// goes idle, and the next writer, small or released from its wait, resets
// the log. The contract has two regimes:
//   - The disk copies the frames the writers add during a pass within
//     walFlowWaitMax: the gap closes, the log resets, and unless a reader
//     pins it the log stays under walHoldMultiple times its bound plus one
//     commit of each large writer in flight at once and one commit of each
//     small writer per walTurnHold while the passes copy them.
//   - The disk cannot: the gap pass is not tried or is cut at its deadline,
//     the log grows until the writers pause, and every wait stays within
//     its bound: a small commit's one hold of the turn, and a large
//     commit's copy or walFlowWaitMax, whichever is longer.
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
	// of one log frame, zero until read. Both only under the writer turn.
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
	// copied is round when a pass last copied the whole log as it began.
	copied uint64
	// pinned reports that the checkpointer's last pass found a reader
	// pinning the log.
	pinned bool
	// idle is closed and replaced each time the checkpointer goes idle or
	// copied moves.
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

// walFlowWaitMax is how long a wait lasts at most once its commit is copied.
// It is longer than the gap takes to close behind a few small writers, about
// 100 to 230 ms for an 8 MiB commit on a Mac.
const walFlowWaitMax = 10 * walTurnHold

// wait waits until the checkpointer goes idle after a pass that began after
// commit seq, until such a pass copied the whole log and walFlowWaitMax has
// passed, or until ctx ends.
func (f *walFlow) wait(ctx context.Context, seq uint64) {
	start := time.Now()
	defer func() { f.stats.addFlowWait(time.Since(start)) }()
	var bound <-chan time.Time
	for {
		// End once idle or pinned. Once the commit is copied, arm the
		// deadline for the rest of walFlowWaitMax.
		f.mtx.Lock()
		settled, pinned, copied, idle := f.settled, f.pinned, f.copied, f.idle
		f.mtx.Unlock()
		if settled >= seq || pinned {
			return
		}
		if copied >= seq && bound == nil {
			left := walFlowWaitMax - time.Since(start)
			if left <= 0 {
				return
			}
			deadline := time.NewTimer(left)
			defer deadline.Stop()
			bound = deadline.C
		}

		// Wait for the checkpointer, the deadline, or the caller.
		select {
		case <-ctx.Done():
			return
		case <-bound:
			return
		case <-idle:
		}
	}
}

// pass marks a checkpointer pass beginning. The pass copies every commit
// counted so far.
func (f *walFlow) pass() {
	f.round = f.commits.Load()
}

// caught marks the pass that began at the last call to pass as having copied
// the whole log as it began, so every commit counted then is copied.
func (f *walFlow) caught() {
	// Wake the waiters only when the copied round moved.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.round <= f.copied {
		return
	}
	f.copied = f.round
	close(f.idle)
	f.idle = make(chan struct{})
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
