package volume_controller

import (
	"context"
	"time"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	volume "github.com/s4wave/spacewave/db/volume"
)

// runGCSweep runs the periodic GC sweep goroutine.
// It waits for the volume to become ready, then prefers the WAL-backed
// concurrent GC manager when the volume provides the required hooks.
// Otherwise it falls back to the legacy unreferenced-node collector.
func (c *Controller) runGCSweep(ctx context.Context) error {
	interval, err := c.config.ParseGCIntervalDur()
	if err != nil {
		c.le.WithError(err).Warn("invalid gc_interval_dur, using default")
		interval = defaultGCInterval
	}
	if interval == 0 {
		c.le.Debug("gc sweep disabled (interval=0)")
		return nil
	}

	vol, err := c.GetVolume(ctx)
	if err != nil {
		return err
	}

	type gcManagerHooksProvider interface {
		GetGCManagerHooks() (block_gc.ManagerHooks, bool)
	}
	if provider, ok := vol.(gcManagerHooksProvider); ok {
		if hooks, ok := provider.GetGCManagerHooks(); ok &&
			hooks.Graph != nil &&
			hooks.ReplayWAL != nil &&
			hooks.AcquireSTW != nil {
			manager := block_gc.NewManager(block_gc.ManagerConfig{
				SweepConfig: block_gc.SweepConfig{
					Graph:      hooks.Graph,
					Target:     volumeSweepTarget{vol: vol},
					ReplayWAL:  hooks.ReplayWAL,
					AcquireSTW: hooks.AcquireSTW,
				},
				SweepInterval: interval,
			})
			c.le.WithField("interval", interval.String()).Debug("gc manager routine started")
			return manager.Run(ctx)
		}
	}

	rg := vol.GetRefGraph()
	if rg == nil {
		c.le.Debug("volume has no ref graph, gc sweep disabled")
		return nil
	}

	collector := block_gc.NewCollector(rg, vol, nil)
	c.le.WithField("interval", interval.String()).Debug("gc sweep routine started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}

		stats, err := collector.Collect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.le.WithError(err).Warn("gc sweep failed")
			continue
		}
		if stats.NodesSwept > 0 {
			c.le.
				WithField("swept", stats.NodesSwept).
				WithField("duration", stats.Duration.String()).
				Info("gc sweep completed")
		}
	}
}

// volumeSweepTarget adapts a volume to the concurrent sweep target interface.
// Object nodes represent world objects that have already been deleted from the
// backing store before becoming sweep candidates, so only graph cleanup remains.
type volumeSweepTarget struct {
	vol volume.Volume
}

// DeleteBlock removes a block from the current volume block store.
func (t volumeSweepTarget) DeleteBlock(ctx context.Context, iri string) error {
	ref, ok := block_gc.ParseBlockIRI(iri)
	if !ok {
		return nil
	}
	return t.vol.RmBlock(ctx, ref)
}

// DeleteObject performs no backend action for object nodes.
func (t volumeSweepTarget) DeleteObject(_ context.Context, _ string) error {
	return nil
}

var _ block_gc.SweepTarget = volumeSweepTarget{}
