package pace

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// evictBackoffMax bounds how long one batch waits for flushes to recover.
	// The batch then runs anyway, so a disk that stays slow still sheds
	// blocks before it fills.
	evictBackoffMax = 30 * time.Second
	// evictBackoffStep is the first back-off wait, doubled up to
	// evictBackoffStepMax.
	evictBackoffStep    = time.Second
	evictBackoffStepMax = 8 * time.Second
	// paceReport is how often the pace logs the time it held eviction.
	paceReport = time.Minute
)

// Pace paces the block store's eviction passes against its database's
// commits: it caps the bytes a second it drops, backs off while flushes run
// slow or commits wait for a checkpoint, and can pause eviction outright.
type Pace struct {
	le *logrus.Entry
	// stats reads the database's stats; false while none is open.
	stats func() (Stats, bool)
	// paused holds every pass until it ends.
	paused bool
	// rate caps the bytes evicted a second, zero for no cap.
	rate float64
	// flushMax is the mean flush time past which a batch waits, zero to
	// never wait on flushes.
	flushMax time.Duration
	// sleep waits d or until ctx ends.
	sleep func(ctx context.Context, d time.Duration) error

	mtx sync.Mutex
	// last is when the last batch was admitted.
	last time.Time
	// prev holds the stats at the last sample, if sampled.
	prev    Stats
	sampled bool
	// held is the time waited since the last report, and reported when.
	held     time.Duration
	reported time.Time
}

// New constructs an optional policy for a store's EvictAdmit hook. The
// policy delays work only; it never changes which blocks may be removed.
func New(le *logrus.Entry, stats func() (Stats, bool), config Config) *Pace {
	return &Pace{le: le, stats: stats, paused: config.Paused, rate: config.BytesPerSecond, flushMax: config.FlushMax, sleep: sleepCtx}
}

// Admit waits before an eviction batch, as block_store_writeback.EvictAdmit
// describes: while paused, until the rate cap allows the bytes the last batch
// dropped, and while the database's flushes run slow, up to evictBackoffMax.
func (p *Pace) Admit(ctx context.Context, lastBytes int64) error {
	// Hold the pass while paused.
	if p.paused {
		<-ctx.Done()
		return context.Cause(ctx)
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	start := time.Now()

	// Space batches to the rate cap.
	if p.rate > 0 && lastBytes > 0 && !p.last.IsZero() {
		due := p.last.Add(time.Duration(float64(lastBytes) / p.rate * float64(time.Second)))
		if wait := time.Until(due); wait > 0 {
			if err := p.sleep(ctx, wait); err != nil {
				return err
			}
		}
	}

	// Back off while flushes run slow, up to evictBackoffMax.
	step := evictBackoffStep
	for waited := time.Duration(0); waited < evictBackoffMax && p.slow(); {
		if err := p.sleep(ctx, step); err != nil {
			return err
		}
		waited += step
		step = min(step*2, evictBackoffStepMax)
	}

	// Log the time held each minute it held any.
	now := time.Now()
	p.last = now
	p.held += now.Sub(start)
	if now.Sub(p.reported) >= paceReport {
		if p.held >= time.Second {
			p.le.WithField("held", p.held.Round(time.Second).String()).Info("store eviction paced")
		}
		p.held, p.reported = 0, now
	}
	return nil
}

// slow samples the database's stats and reports whether, since the last
// sample, its mean flush ran past flushMax or a commit waited for a
// checkpoint.
func (p *Pace) slow() bool {
	// Take the change since the last sample.
	if p.flushMax == 0 {
		return false
	}
	st, ok := p.stats()
	if !ok {
		return false
	}
	prev, sampled := p.prev, p.sampled
	p.prev, p.sampled = st, true
	if !sampled || prev.Flushes > st.Flushes {
		return false
	}

	// Compare it to the bounds.
	if st.BuildWaits > prev.BuildWaits {
		return true
	}
	flushes := st.Flushes - prev.Flushes
	if flushes == 0 {
		return false
	}
	return (st.FlushTime-prev.FlushTime)/time.Duration(flushes) > p.flushMax
}

// sleepCtx waits d or until ctx ends.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-t.C:
		return nil
	}
}
