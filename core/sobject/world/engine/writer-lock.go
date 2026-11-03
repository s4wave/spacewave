package sobject_world_engine

import (
	"context"
	"os"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/csync"
	"github.com/sirupsen/logrus"
)

// slowHoldThreshold is how long a holder may keep the World writer lock
// before its release logs the holder's name and the hold. Every World write
// waits for the lock, so a long hold is what a slow commit's lock-wait
// stage waited on. Tests set it.
var slowHoldThreshold = time.Second

// stuckHoldThreshold is how long a hold runs before the lock writes every
// goroutine's stack to a file and logs its path, so a hold that a release
// only reports afterwards can be traced to what the holder waited on.
// Tests set it.
var stuckHoldThreshold = 10 * time.Second

// stuckDumpInterval spaces the stack dumps, since each one walks every
// goroutine. Tests set it.
var stuckDumpInterval = 10 * time.Minute

// writerLock is the World writer lock. Each holder names itself, so a slow
// hold is logged with what held it.
type writerLock struct {
	// mtx is the lock.
	mtx csync.Mutex
	// le logs slow holds, or nil to log none.
	le *logrus.Entry

	// dumpMtx guards lastDump.
	dumpMtx sync.Mutex
	// lastDump is when the last stuck hold dumped the stacks.
	lastDump time.Time
}

// Lock takes the lock for holder and returns the function that releases it.
// The release logs the hold when it exceeded slowHoldThreshold; calls after
// the first do nothing.
func (l *writerLock) Lock(ctx context.Context, holder string) (func(), error) {
	// Take the lock and note when.
	unlock, err := l.mtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	var stuck *time.Timer
	if l.le != nil {
		stuck = time.AfterFunc(stuckHoldThreshold, func() { l.dumpStuck(holder, start) })
	}

	// Release it once, logging a slow hold.
	var released atomic.Bool
	return func() {
		// Stop diagnostics and release this hold only once.
		if released.Swap(true) {
			return
		}
		if stuck != nil {
			stuck.Stop()
		}
		held := time.Since(start)
		unlock()
		if held > slowHoldThreshold && l.le != nil {
			l.le.WithFields(logrus.Fields{
				"holder": holder,
				"held":   held,
			}).Warn("slow World writer lock hold")
		}
	}, nil
}

// dumpStuck writes every goroutine's stack to a temporary file while holder
// still holds the lock, and logs the file. It dumps at most once each
// stuckDumpInterval.
func (l *writerLock) dumpStuck(holder string, start time.Time) {
	// Skip the dump when one ran recently.
	l.dumpMtx.Lock()
	now := time.Now()
	if !l.lastDump.IsZero() && now.Sub(l.lastDump) < stuckDumpInterval {
		l.dumpMtx.Unlock()
		return
	}
	l.lastDump = now
	l.dumpMtx.Unlock()

	// Write the grouped stacks.
	le := l.le.WithFields(logrus.Fields{"holder": holder, "held": now.Sub(start)})
	f, err := os.CreateTemp("", "world-writer-hold-*.txt")
	if err != nil {
		le.WithError(err).Warn("World writer lock held long; could not write the goroutine stacks")
		return
	}
	defer f.Close()
	if err := pprof.Lookup("goroutine").WriteTo(f, 1); err != nil {
		le.WithError(err).Warn("World writer lock held long; could not write the goroutine stacks")
		return
	}
	le.WithField("stacks", f.Name()).Warn("World writer lock held long; wrote the goroutine stacks")
}
