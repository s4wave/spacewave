package block_store_fillcache

import (
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// rateLogInterval is the least time between two lines of a rateLog.
const rateLogInterval = time.Minute

// rateLog logs warnings at most once per rateLogInterval, so a cache that fails
// on every read or write logs a line a minute and not a line per block.
type rateLog struct {
	// le is the entry the warnings go to.
	le *logrus.Entry
	// last is the Unix nanosecond time of the last line logged.
	last atomic.Int64
	// suppressed counts the warnings dropped since the last line.
	suppressed atomic.Uint64
}

// warn logs msg with err unless a line went out within rateLogInterval. The
// next line to go out carries the count of the warnings dropped before it.
func (r *rateLog) warn(err error, msg string) {
	// Drop the warning when a line went out recently.
	now := time.Now().UnixNano()
	last := r.last.Load()
	if now-last < int64(rateLogInterval) || !r.last.CompareAndSwap(last, now) {
		r.suppressed.Add(1)
		return
	}

	// Log the warning with the number of dropped ones.
	r.le.WithError(err).WithField("suppressed", r.suppressed.Swap(0)).Warn(msg)
}
