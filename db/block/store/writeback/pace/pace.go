package pace

import (
	"context"
	"math"
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
	// setMtx guards paused, rate and changed.
	setMtx sync.Mutex
	// paused holds every batch until it is cleared or its pass ends;
	// SetPaused changes it while passes run.
	paused bool
	// rate caps the bytes evicted a second, zero for no cap;
	// SetBytesPerSecond changes it while passes run.
	rate float64
	// changed is closed and replaced when paused or rate changes, waking a
	// batch waiting under the old setting to read the new one.
	changed chan struct{}
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
	// Apply the config through the setters, which open the first change channel.
	p := &Pace{le: le, stats: stats, sleep: sleepCtx}
	p.SetPaused(config.Paused)
	p.SetBytesPerSecond(config.BytesPerSecond)
	p.SetFlushMax(config.FlushMax)
	return p
}

// SetBytesPerSecond sets the cap on the bytes evicted a second, zero or less
// for no cap. A batch already waiting on the old cap recomputes its wait from
// the new one, so lifting a near-zero cap releases it at once.
func (p *Pace) SetBytesPerSecond(rate float64) {
	p.set(func() { p.rate = max(rate, 0) })
}

// SetPaused pauses or resumes eviction. A pause holds each pass at its next
// batch, including one waiting on the rate cap or backing off; a batch
// already admitted runs. Resuming admits a held batch at once, with no wait
// for the time it was held.
func (p *Pace) SetPaused(paused bool) {
	p.set(func() { p.paused = paused })
}

// set applies change to the settings and wakes every batch waiting under the
// old ones.
func (p *Pace) set(change func()) {
	// Close the channel the waiters hold and open the next one.
	p.setMtx.Lock()
	defer p.setMtx.Unlock()
	change()
	if p.changed != nil {
		close(p.changed)
	}
	p.changed = make(chan struct{})
}

// settings returns the pause, the rate cap and the channel their next change
// closes.
func (p *Pace) settings() (bool, float64, <-chan struct{}) {
	p.setMtx.Lock()
	defer p.setMtx.Unlock()
	return p.paused, p.rate, p.changed
}

// SetFlushMax sets the mean flush time past which a batch backs off, zero or
// less to never back off. A batch backing off reads it at its next sample.
func (p *Pace) SetFlushMax(d time.Duration) {
	p.flushMax.Store(int64(max(d, 0)))
}

// Admit waits before an eviction batch, as block_store_writeback.EvictAdmit
// describes: while paused, until the rate cap allows the bytes the last batch
// dropped, and while the database's flushes run slow, up to evictBackoffMax.
// A change to the pause or the cap during any of these waits starts them
// again under the new settings.
func (p *Pace) Admit(ctx context.Context, lastBytes int64) error {
	// Let a queued eviction leave without waiting for another pass's timer.
	release, err := p.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Time the whole wait, across every restart, for the report.
	start := time.Now()

	// Wait until every rule admits the batch; a settings change restarts the rules.
admit:
	for {
		// Hold the batch while paused.
		paused, rate, changed := p.settings()
		if paused {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case <-changed:
				continue
			}
		}

		// Space batches to the rate cap, by the bytes the last one dropped.
		if rate > 0 && lastBytes > 0 && !p.last.IsZero() {
			if wait := time.Until(p.last.Add(spacing(lastBytes, rate))); wait > 0 {
				woken, err := p.sleep(ctx, wait, changed)
				if err != nil {
					return err
				}
				if woken {
					continue
				}
			}
		}

		// Back off while flushes run slow, up to evictBackoffMax.
		step := evictBackoffStep
		for waited := time.Duration(0); waited < evictBackoffMax && p.slow(); {
			woken, err := p.sleep(ctx, step, changed)
			if err != nil {
				return err
			}
			if woken {
				continue admit
			}
			waited += step
			step = min(step*2, evictBackoffStepMax)
		}
		break
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

// spacing returns the time bytes take at rate bytes a second, saturating at
// the longest duration rather than overflowing to a negative one.
func spacing(bytes int64, rate float64) time.Duration {
	d := float64(bytes) / rate * float64(time.Second)
	if d >= math.MaxInt64 {
		return math.MaxInt64
	}
	return time.Duration(d)
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
