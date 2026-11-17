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
// that began after the commit. The checkpointer goes idle only with the log
// under walHoldMultiple times its bound, which past it means SQLite reset
// the log; once a pass copied the whole log, which the next writer to begin
// resets, and that writer may be the one waiting; once a reader pins it;
// once a pass fails, which no wait can mend; once a reader it does not judge
// a pin cuts a pass short; or once another checkpointer on the same log
// makes a pass busy. In the last two the commit's frames may be left
// uncopied, and later passes copy them. A log a reader pins cannot reset, so
// while the last pass found it pinned no commit waits, and the finding ends
// every wait at once. The wait's length is the copy of the frames behind the
// commit, at most the commit's own frames plus those of the commits in
// flight beside it, over the copy rate: about 24 MiB/s on forge, so 2.2 s
// for a 54 MB commit.
//
// The wait also ends once a pass that began after the commit copied the
// whole log as it began, and the wait has lasted walFlowWaitMax: by then the
// commit is copied, and holding it on serves only to let the gap close, which
// small writers that together add more than walGapFrames frames per pass
// never let happen. So the wait is at most the copy of its frames or
// walFlowWaitMax, whichever is longer. On a disk that cannot copy the gap
// within walFlowWaitMax the checkpointer never goes idle, and this deadline
// is what ends the wait.
//
// Invariant: a commit of at most walGapFrames frames never waits here. It
// waits only behind the checkpointer's hold of the writer turn, at most one
// per commit: a back-pressure slice of walTurnHold, or the gap pass of at
// most walGapHold (walFlowWaitMax) and the flush of the pages it copied.
//
// Several writers: past walHoldMultiple times the bound the checkpointer
// goes idle only once a pass copied the whole log, and each pass holds the
// writers back from its start. Between two slices of the hold it admits
// waiting writers in arrival order until their commits spend a budget of the
// frames the measured copy rate moves in one walTurnHold, and admits a
// commit larger than that alone. A large writer that commits there waits, so
// once every large writer waits, only small writers' commits land between
// passes; the first caught-up pass after them finds new frames it can copy
// within walFlowWaitMax, takes the turn ahead of the writers, since the log
// is past walGapQueueMultiple times its bound, and closes the gap; the
// checkpointer goes idle, and the next writer, small or released from its
// wait, resets the log. The contract has two regimes:
//   - The writers together add less than the disk copies: the gap closes,
//     the log resets, and unless a reader pins it the log stays under
//     walHoldMultiple times its bound plus one commit of each large writer
//     in flight at once and one budget per walTurnHold while the passes copy
//     them. A small commit waits behind at most one hold of the turn.
//   - They add more: the slice budget paces every writer to the disk, so the
//     frames that land during a held pass are no more than it copies and the
//     log stays bounded by that pacing. A large commit waits in the flow at
//     most the copy of its own frames plus walFlowWaitMax, under any number
//     of small writers. A small commit waits for the frames queued ahead of it,
//     at most one commit of each writer, at the copy rate, plus one hold.
//     Without the budget each slice admitted one commit per writer, so many
//     small writers outran the disk, every pass was longer than the last,
//     and a large commit's wait grew without bound.
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
	// copied is round when a pass last copied the whole log as it began.
	copied uint64
	// pinned reports that the checkpointer's last pass found a reader
	// pinning the log.
	pinned bool
	// idle is closed and replaced each time the checkpointer goes idle or
	// copied moves.
	idle chan struct{}
	// left is the frame budget the commits since a held pass's last slice
	// may still spend, and spent is closed once they spend it; spent is nil
	// while no budget is armed.
	left  int64
	spent chan struct{}

	// armed reports that spent is set, so a commit outside a held pass
	// charges nothing but an atomic load.
	armed atomic.Bool
}

// newWALFlow returns the flow of a store whose log is bound at bound bytes.
func newWALFlow(size func() int64, wake func(), bound int64, stats *storeStats) *walFlow {
	return &walFlow{size: size, wake: wake, hold: walHoldMultiple * bound, stats: stats, idle: make(chan struct{})}
}

// outran reports whether the commit that conn just made left the log at
// walHoldMultiple times its bound with more than walGapFrames frames added,
// and charges its frames to the slice budget armed while a held pass waits
// for the turn. It runs under the writer turn, after COMMIT, and costs one
// stat of the log, an atomic load, and, on a store's first commit, one read
// of its page size.
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

	// Count the added bytes in frames of the store's page, rounding up.
	if f.frameBytes == 0 {
		var page int64
		if err := conn.QueryRowContext(context.Background(), "PRAGMA page_size").Scan(&page); err != nil || page <= 0 {
			return false
		}
		f.frameBytes = page + walFrameHeaderBytes
	}
	frames := (added + f.frameBytes - 1) / f.frameBytes
	f.charge(frames)
	return size >= f.hold && frames > walGapFrames
}

// arm arms a slice budget of frames for the commits that follow and returns
// the channel closed once they spend it.
func (f *walFlow) arm(frames int64) <-chan struct{} {
	// Set the budget and the channel its spending closes.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.left, f.spent = frames, make(chan struct{})
	f.armed.Store(true)
	return f.spent
}

// disarm drops the armed budget, if any.
func (f *walFlow) disarm() {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.spent = nil
	f.armed.Store(false)
}

// charge spends frames of the armed budget, closing spent once it is used.
func (f *walFlow) charge(frames int64) {
	// Skip the lock while no budget is armed.
	if !f.armed.Load() {
		return
	}

	// Spend the frames, and close spent once the budget is used.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.spent == nil {
		return
	}
	if f.left -= frames; f.left <= 0 {
		close(f.spent)
		f.spent = nil
		f.armed.Store(false)
	}
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
		// End once idle or pinned; once the commit is copied, arm the
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

// pass marks a checkpointer pass beginning; it copies every commit counted
// so far.
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
