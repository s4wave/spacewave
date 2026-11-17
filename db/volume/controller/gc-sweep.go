package volume_controller

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume"
)

// stallWarnAfter is how long collection passes must keep reporting a stalled
// free before the sweep warns of it, and how often it warns again while the
// stall lasts. A wait on the hold mark lasts about one grace period after a
// Land that held a block at zero, so twice the default grace warns only of a
// stall that persists.
const stallWarnAfter = 2 * block_gc.DefaultGrace

// stallWatch follows one kind of stalled free across collection passes. It
// is clear until a pass reports the stall, then stalled since that pass's
// time, until a pass that ran every step reports none, or a pass that
// stopped early reports none stallWarnAfter after the last report: a pass
// that stopped with work left may not have reached the free, but a stall
// no pass reported for that long has ended, and a later one starts anew.
type stallWatch struct {
	// since is when the stall began, zero while clear.
	since time.Time
	// last is when a pass last reported the stall.
	last time.Time
	// warned is when the sweep last warned of this stall.
	warned time.Time
}

// observe records the stall the pass that ended at now reported, and returns
// how long the stall has lasted and whether to warn of it: once it has lasted
// stallWarnAfter, then each stallWarnAfter while it lasts.
func (s *stallWatch) observe(now time.Time, stalled, more bool) (time.Duration, bool) {
	// Clear the stall on a pass that ran every step without it, and on one
	// that stopped early once the last report aged.
	if !stalled {
		if !more || now.Sub(s.last) >= stallWarnAfter {
			*s = stallWatch{}
		}
		return 0, false
	}

	// Start the stall, and warn once it lasted and the last warning aged.
	if s.since.IsZero() {
		s.since = now
	}
	s.last = now
	lasted := now.Sub(s.since)
	if lasted < stallWarnAfter || now.Sub(s.warned) < stallWarnAfter {
		return lasted, false
	}
	s.warned = now
	return lasted, true
}

// gcDelegator is implemented by a volume that views another controller's
// storage. That controller sees every owner's references and sweeps through the
// storage's own block store, so this controller tracks references and never
// collects.
type gcDelegator interface {
	// DelegatesGC marks the volume as a view of collected storage.
	DelegatesGC()
}

// storageMaintainer is implemented by a volume whose storage reclaims the
// space of removed blocks in a separate step.
type storageMaintainer interface {
	// Maintenance reclaims the space of removed blocks.
	Maintenance(ctx context.Context) error
}

// runGCSweep runs the periodic collection goroutine. It waits for the volume
// to become ready, then collects on each interval, at once while a pass stops
// with work left, and at once when the volume enters its disk reserve.
func (c *Controller) runGCSweep(ctx context.Context) error {
	// Choose the configured collection interval or disable volume collection.
	interval, err := c.config.ParseGCIntervalDur()
	if err != nil {
		c.le.WithError(err).Warn("invalid gc_interval_dur, using default")
		interval = defaultGCInterval
	}
	if interval == 0 || c.config.GCDisabled() {
		c.le.Debug("gc disabled")
		return nil
	}

	// Wait for the volume, and leave collection to the controller of the
	// storage a view reads, and to volumes that collect.
	vol, err := c.GetVolume(ctx)
	if err != nil {
		return err
	}
	if _, ok := vol.(gcDelegator); ok {
		c.le.Debug("volume delegates gc, sweep disabled")
		return nil
	}
	collector, ok := vol.(volume.Collector)
	if !ok {
		c.le.Debug("volume does not collect, sweep disabled")
		return nil
	}

	// The collector is background work: its commits wait for a checkpoint
	// the volume's writes outpaced, and its scans leave the index cache.
	ctx = block.WithBackground(ctx)
	timer := time.NewTimer(interval)
	defer timer.Stop()
	hw, _ := vol.(volume.HealthWatcher)
	reserve := &reserveWatch{hw: hw}
	var saturated, held stallWatch
	for {
		// Pull the pass in when the volume entered the reserve.
		entered, healthCh := reserve.entered()
		if entered {
			c.le.Warn("volume below its disk reserve, collecting now")
			timer.Reset(0)
		}

		// Wait for the next collection deadline, a health change or volume
		// cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-healthCh:
			continue
		case <-timer.C:
		}

		// Collect, retrying a failure at the next deadline.
		res, err := collector.Collect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.le.WithError(err).Warn("gc pass failed")
			timer.Reset(interval)
			continue
		}

		// Report a pass that changed anything, and warn of underflows.
		if res.Netted != 0 || res.Freed != 0 || res.Lapsed != 0 {
			c.le.
				WithField("netted", res.Netted).
				WithField("counted", res.Counted).
				WithField("queued", res.Queued).
				WithField("freed", res.Freed).
				WithField("kept", res.Kept).
				WithField("tail-saturated", res.TailSaturated).
				WithField("stall-hold-mark", res.StallHoldMark).
				WithField("leaked", res.Leaked).
				WithField("lapsed", res.Lapsed).
				WithField("more", res.More).
				WithField("duration", res.Duration.String()).
				Debug("gc pass completed")
		}
		if res.Underflows != 0 {
			c.le.WithField("underflows", res.Underflows).Warn("gc pass refused count changes below zero")
		}
		if res.Stranded != 0 {
			c.le.WithField("stranded", res.Stranded).Warn("gc pass dropped the releases of blocks lost with their child lists")
		}

		// Warn of a free that stays stalled pass after pass.
		now := time.Now()
		if lasted, warn := saturated.observe(now, res.TailSaturated != 0, res.More); warn {
			c.le.
				WithField("tail-saturated", res.TailSaturated).
				WithField("stalled-for", lasted.Round(time.Second).String()).
				Warn("gc frees kept: the journal past the cursor names more blocks than the tail index holds")
		}
		if lasted, warn := held.observe(now, res.StallHoldMark != 0, res.More); warn {
			c.le.
				WithField("stall-hold-mark", res.StallHoldMark).
				WithField("hold-mark", res.HoldMark).
				WithField("stalled-for", lasted.Round(time.Second).String()).
				Warn("gc frees wait on the hold mark: writers keep holding blocks the collector has at zero")
		}

		// Let the storage reclaim the space of the freed blocks.
		if res.Freed != 0 {
			if m, ok := vol.(storageMaintainer); ok {
				if err := m.Maintenance(ctx); err != nil && ctx.Err() == nil {
					c.le.WithError(err).Warn("storage maintenance failed")
				}
			}
		}
		if res.More {
			timer.Reset(0)
		} else {
			timer.Reset(interval)
		}
	}
}
