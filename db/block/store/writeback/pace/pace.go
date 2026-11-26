package pace

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/csync"
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
	// rateMtx guards rate and rateSet.
	rateMtx sync.Mutex
	// rate caps the bytes evicted a second, zero for no cap;
	// SetBytesPerSecond changes it while passes run.
	rate float64
	// rateSet is closed and replaced when the rate changes, waking a batch
	// spacing itself by the old rate.
	rateSet chan struct{}
	// flushMax is the mean flush time past which a batch waits, zero to
	// never wait on flushes; SetFlushMax changes it while passes run.
	flushMax atomic.Int64
	// sleep waits d, until wake closes, or until ctx ends, and reports
	// whether wake ended it. A nil wake never closes.
	sleep func(ctx context.Context, d time.Duration, wake <-chan struct{}) (bool, error)

	mtx csync.Mutex
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
	p := &Pace{le: le, stats: stats, paused: config.Paused, sleep: sleepCtx, rateSet: make(chan struct{})}
	p.SetBytesPerSecond(config.BytesPerSecond)
	p.SetFlushMax(config.FlushMax)
	return p
}

// SetBytesPerSecond sets the cap on the bytes evicted a second, zero or less
// for no cap. A batch already waiting on the old cap recomputes its wait from
// the new one, so lifting a near-zero cap releases it at once.
func (p *Pace) SetBytesPerSecond(rate float64) {
	// Store the cap and wake every batch waiting on the old one.
	p.rateMtx.Lock()
	defer p.rateMtx.Unlock()
	p.rate = max(rate, 0)
	if p.rateSet != nil {
		close(p.rateSet)
	}
	p.rateSet = make(chan struct{})
}

// currentRate returns the rate cap and the channel its next change closes.
func (p *Pace) currentRate() (float64, <-chan struct{}) {
	p.rateMtx.Lock()
	defer p.rateMtx.Unlock()
	return p.rate, p.rateSet
}

// SetFlushMax sets the mean flush time past which a batch backs off, zero or
// less to never back off. A batch backing off reads it at its next sample.
func (p *Pace) SetFlushMax(d time.Duration) {
	p.flushMax.Store(int64(max(d, 0)))
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

	// Let a queued eviction leave without waiting for another pass's timer.
	release, err := p.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	start := time.Now()

	// Space batches to the rate cap, recomputing the due time from the last
	// admission whenever the cap changes during the wait.
	for lastBytes > 0 && !p.last.IsZero() {
		rate, changed := p.currentRate()
		if rate == 0 {
			break
		}
		due := p.last.Add(time.Duration(float64(lastBytes) / rate * float64(time.Second)))
		wait := time.Until(due)
		if wait <= 0 {
			break
		}
		woken, err := p.sleep(ctx, wait, changed)
		if err != nil {
			return err
		}
		if !woken {
			break
		}
	}

	// Back off while flushes run slow, up to evictBackoffMax.
	step := evictBackoffStep
	for waited := time.Duration(0); waited < evictBackoffMax && p.slow(); {
		if _, err := p.sleep(ctx, step, nil); err != nil {
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
	flushMax := time.Duration(p.flushMax.Load())
	if flushMax == 0 {
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
	return float64(st.FlushTime-prev.FlushTime)/float64(flushes) > float64(flushMax)
}

// sleepCtx waits d, until wake closes, or until ctx ends, and reports whether
// wake ended it.
func sleepCtx(ctx context.Context, d time.Duration, wake <-chan struct{}) (bool, error) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false, context.Cause(ctx)
	case <-wake:
		return true, nil
	case <-t.C:
		return false, nil
	}
}
