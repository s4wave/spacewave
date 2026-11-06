package volume_controller

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
)

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
