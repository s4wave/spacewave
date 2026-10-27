package volume_controller

import (
	"context"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
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

// markIntervals is the number of collection intervals between block passes,
// before the collector backs off after passes that find little.
const markIntervals = 15

// quietIntervals is the number of collection intervals after the volume opens
// before a block pass may start. A process that just opened serves its first
// readers from a cold store, and a pass due during the restart would read the
// whole graph beside them.
const quietIntervals = 5

// ownerTracer is implemented by a reference graph that can trace what owns a
// block, as RefGraph.TraceOwners describes.
type ownerTracer interface {
	// TraceOwners finds the owned path to each block.
	TraceOwners(ctx context.Context, blocks []string, maxDepth int) ([]block_gc.OwnerTrace, error)
}

// traceDepth bounds the levels a trace of unknown blocks walks up.
const traceDepth = 64

// slowSweepTx is the sweep transaction time past which a pass warns. A sweep
// transaction holds the writer every World commit waits on, and its budget
// keeps it near 50 ms.
const slowSweepTx = 500 * time.Millisecond

// runGCSweep runs the periodic GC sweep goroutine. It waits for the volume to
// become ready, then collects its reference graph on each interval.
func (c *Controller) runGCSweep(ctx context.Context) error {
	// Choose the configured collection interval or disable volume collection.
	interval, err := c.config.ParseGCIntervalDur()
	if err != nil {
		c.le.WithError(err).Warn("invalid gc_interval_dur, using default")
		interval = defaultGCInterval
	}
	if interval == 0 {
		c.le.Debug("gc disabled (interval=0)")
		return nil
	}

	// Wait for the volume before choosing its collection mechanism.
	vol, err := c.GetVolume(ctx)
	if err != nil {
		return err
	}

	// Leave collection to the controller of the storage a view reads.
	if _, ok := vol.(gcDelegator); ok {
		c.le.Debug("volume delegates gc, sweep disabled")
		return nil
	}

	// Require a reference graph for collection.
	rg := vol.GetRefGraph()
	if rg == nil {
		c.le.Debug("volume has no ref graph, gc sweep disabled")
		return nil
	}

	// Construct the collector for the volume reference graph. A block pass
	// reads the whole graph, so it runs markIntervals intervals after the
	// last one ended, or up to 8x that after passes that find little. The
	// schedule resumes from the volume, so a pass a restart cut short or
	// postponed runs once the quiet start is over.
	collector := block_gc.NewCollector(rg, vol, nil)
	next, err := readGcSchedule(ctx, vol)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.le.WithError(err).Warn("gc schedule unreadable, scheduling the block pass from now")
	}
	if quiet := time.Now().Add(quietIntervals * interval); !next.IsZero() && next.Before(quiet) {
		next = quiet
	}
	collector.SetMarkInterval(markIntervals*interval, next)
	c.le.WithField("interval", interval.String()).Debug("gc sweep routine started")

	// Collect unreferenced volume nodes on each maintenance deadline, for at
	// most one interval at a time, then a due block pass, which sweeps all it
	// marked however long that takes. A collection that stops with candidates
	// left continues at once.
	// A volume entering its disk reserve starts a block pass at once, since
	// only collection frees the space its writes wait on.
	timer := time.NewTimer(interval)
	defer timer.Stop()
	var traced bool
	hw, _ := vol.(volume.HealthWatcher)
	reserve := &reserveWatch{hw: hw}
	for {
		// Pull the block pass in when the volume entered the reserve.
		entered, healthCh := reserve.entered()
		if entered {
			c.le.Warn("volume below its disk reserve, collecting now")
			collector.SetMarkInterval(markIntervals*interval, time.Now())
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

		// Reap expired volume root pins before collecting orphaned nodes.
		if reaper, ok := vol.(interface{ ReapRootPins(context.Context) error }); ok {
			if err := reaper.ReapRootPins(ctx); err != nil {
				return err
			}
		}

		// Collect unreferenced nodes, retrying failures at the next deadline.
		stats, err := collector.CollectFor(ctx, interval)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.le.WithError(err).Warn("gc sweep failed")
			timer.Reset(interval)
			continue
		}

		// Report each collection that found candidates or marked, warn of a
		// pass that blocks of unknown references stopped, and continue a
		// collection the budget stopped.
		if stats.UnreferencedNodeCount != 0 || stats.BlockPasses != 0 || stats.MinorPasses != 0 {
			le := c.le
			if stats.MinorPasses != 0 {
				le = le.WithField("minor", true).WithField("young", stats.YoungBlocks)
			}
			if stats.BlockPasses != 0 || stats.MinorPasses != 0 {
				le = le.
					WithField("roots", stats.MarkRoots).
					WithField("owners", block_gc.FormatOwners(stats.MarkOwners)).
					WithField("rescued", stats.RescuedNodes).
					WithField("absent", stats.AbsentBlocks).
					WithField("sweep-longest-tx", stats.SweepLongestTx.String()).
					WithField("sweep-longest-rescue", stats.SweepLongestRescue.String())
			}
			le.
				WithField("swept", stats.NodesSwept).
				WithField("candidates", stats.Candidates).
				WithField("live", stats.LiveNodes).
				WithField("unreached", stats.UnreachedBlocks).
				WithField("blocks-swept", stats.BlocksSwept).
				WithField("mark-duration", stats.MarkDuration.String()).
				WithField("mark-scans", stats.MarkScans).
				WithField("mark-longest-read", stats.MarkLongestRead.String()).
				WithField("more", stats.More).
				WithField("duration", stats.Duration.String()).
				Info("gc sweep completed")
		}
		// Warn when a minor pass stopped minor passes, once per stop.
		if stats.MinorStopped != "" {
			c.le.
				WithField("reason", stats.MinorStopped).
				Warn("gc minor passes stopped until a block pass promotes")
		}
		if stats.SweepLongestTx >= slowSweepTx {
			c.le.
				WithField("tx", stats.SweepLongestTx.String()).
				WithField("rescue", stats.SweepLongestTxRescue.String()).
				WithField("sweep", (stats.SweepLongestTx - stats.SweepLongestTxRescue).String()).
				Warn("slow sweep transaction")
		}
		if stats.UnknownBlocks != 0 {
			c.le.
				WithField("unknown", stats.UnknownBlocks).
				WithField("sample", block_gc.FormatIRIs(stats.UnknownSample)).
				WithField("named-by", block_gc.FormatIRIs(stats.UnknownNamers)).
				WithField("blocks-swept", stats.BlocksSwept).
				Warn("gc block pass reached blocks of unknown references and stopped sweeping")

			// Trace what owns the sample once per process: each level of a
			// trace scans every child list.
			if tracer, ok := rg.(ownerTracer); ok && !traced && len(stats.UnknownSample) != 0 {
				traced = true
				go c.traceUnknown(ctx, tracer, stats.UnknownSample)
			}
		}
		// Save the next block pass so a restart resumes the schedule.
		if stats.BlockPasses != 0 {
			if err := writeGcSchedule(ctx, vol, collector.NextMark()); err != nil && ctx.Err() == nil {
				c.le.WithError(err).Warn("gc schedule not saved")
			}
		}
		// Let the storage reclaim the space of the swept blocks.
		if stats.NodesSwept != 0 || stats.BlocksSwept != 0 {
			if m, ok := vol.(storageMaintainer); ok {
				if err := m.Maintenance(ctx); err != nil && ctx.Err() == nil {
					c.le.WithError(err).Warn("storage maintenance failed")
				}
			}
		}
		if stats.More {
			timer.Reset(0)
		} else {
			timer.Reset(interval)
		}
	}
}

// gcScheduleStoreID names the volume object store holding the GC schedule.
const gcScheduleStoreID = "volume-gc"

// gcScheduleKey is the GcSchedule key in that store.
var gcScheduleKey = []byte("schedule")

// readGcSchedule returns when the volume's next block pass is due, or zero
// when no pass has saved one.
func readGcSchedule(ctx context.Context, vol volume.Volume) (time.Time, error) {
	// Open the schedule store and read the saved schedule.
	store, release, err := vol.AccessObjectStore(ctx, gcScheduleStoreID, nil)
	if err != nil {
		return time.Time{}, err
	}
	defer release()
	sched := &GcSchedule{}
	err = kvtx.RunTransaction(ctx, false, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, false)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		data, found, err := tx.Get(ctx, gcScheduleKey)
		if err != nil || !found {
			return err
		}
		return sched.UnmarshalVT(data)
	})
	if err != nil || sched.GetNextBlockPass() == nil {
		return time.Time{}, err
	}
	return sched.GetNextBlockPass().AsTime(), nil
}

// writeGcSchedule saves when the volume's next block pass is due.
func writeGcSchedule(ctx context.Context, vol volume.Volume, next time.Time) error {
	// Encode the schedule and write it to the schedule store.
	data, err := (&GcSchedule{NextBlockPass: timestamppb.New(next)}).MarshalVT()
	if err != nil {
		return err
	}
	store, release, err := vol.AccessObjectStore(ctx, gcScheduleStoreID, nil)
	if err != nil {
		return err
	}
	defer release()
	return kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, gcScheduleKey, data)
	})
}

// traceUnknown logs the owner and path that keep each sampled block of unknown
// references live.
func (c *Controller) traceUnknown(ctx context.Context, tracer ownerTracer, sample []string) {
	start := time.Now()
	traces, err := tracer.TraceOwners(ctx, sample, traceDepth)
	if err != nil {
		if ctx.Err() == nil {
			c.le.WithError(err).Warn("gc trace of unknown blocks failed")
		}
		return
	}
	for _, trace := range traces {
		le := c.le.
			WithField("block", trace.Block).
			WithField("owner", trace.Owner).
			WithField("depth", len(trace.Path)).
			WithField("dur", time.Since(start).String())
		if len(trace.Path) != 0 {
			le = le.WithField("top", trace.Path[0])
		}
		if len(trace.Path) > 1 {
			le = le.WithField("namer", trace.Path[len(trace.Path)-2])
		}
		le.Warn("gc traced a block of unknown references")
	}
}
