package volume_controller

import (
	"context"
	"time"

	block_gc "github.com/aperturerobotics/hydra/block/gc"
	kvtx_volume "github.com/aperturerobotics/hydra/volume/common/kvtx"
)

// runGCSweep runs the periodic GC sweep goroutine.
// It waits for the volume to become ready, performs bootstrap if needed,
// then runs Collector.Collect on a configurable interval.
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

	kvVol, ok := vol.(kvtx_volume.KvtxVolume)
	if !ok {
		c.le.Debug("volume does not implement KvtxVolume, gc sweep disabled")
		return nil
	}

	rg := kvVol.GetRefGraph()
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
