package plugin_host_scheduler

import (
	"context"
	"sync"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

type manifestCopyIdentity string

const (
	manifestCopyIdentityExternal manifestCopyIdentity = "external"
	manifestCopyIdentityLocal    manifestCopyIdentity = "local"
)

type manifestCopyCounters struct {
	mtx             sync.Mutex
	readCount       uint64
	readBytes       uint64
	active          *block.ReadCounter
	activeReadCount uint64
	activeReadBytes uint64
}

func (c *manifestCopyCounters) foldActiveLocked() {
	if c == nil || c.active == nil {
		return
	}
	snapshot := c.active.Snapshot()
	if snapshot.BlockReadCount > c.activeReadCount {
		c.readCount += snapshot.BlockReadCount - c.activeReadCount
	}
	if snapshot.BlockReadBytes > c.activeReadBytes {
		c.readBytes += snapshot.BlockReadBytes - c.activeReadBytes
	}
	c.activeReadCount = snapshot.BlockReadCount
	c.activeReadBytes = snapshot.BlockReadBytes
}

func (c *manifestCopyCounters) register(counter *block.ReadCounter) {
	if c == nil || counter == nil {
		return
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.active == counter {
		return
	}
	c.foldActiveLocked()
	c.active = counter
	c.activeReadCount = 0
	c.activeReadBytes = 0
}

func (c *manifestCopyCounters) observe(counter *block.ReadCounter) {
	if c == nil || counter == nil {
		return
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.active == counter {
		c.foldActiveLocked()
	}
}

func (c *manifestCopyCounters) snapshot() (uint64, uint64) {
	if c == nil {
		return 0, 0
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.foldActiveLocked()
	return c.readCount, c.readBytes
}

func (c *manifestCopyCounters) finish(counter *block.ReadCounter) {
	if c == nil || counter == nil {
		return
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.active != counter {
		return
	}
	c.foldActiveLocked()
	c.active = nil
	c.activeReadCount = 0
	c.activeReadBytes = 0
}

type manifestCopyAccounting struct {
	manifestRef         *bucket.ObjectRef
	sourceBucketID      string
	destinationBucketID string
	sourceIdentity      manifestCopyIdentity
	destinationIdentity manifestCopyIdentity
	counters            *manifestCopyCounters
}

func newManifestCopyAccounting(
	manifestSnapshot *bldr_manifest.ManifestSnapshot,
	sourceBucketID, destinationBucketID string,
) *manifestCopyAccounting {
	sourceIdentity := manifestCopyIdentityExternal
	if sourceBucketID == "" || (destinationBucketID != "" && sourceBucketID == destinationBucketID) {
		sourceIdentity = manifestCopyIdentityLocal
	}
	var manifestRef *bucket.ObjectRef
	if manifestSnapshot != nil && manifestSnapshot.GetManifestRef() != nil {
		manifestRef = manifestSnapshot.GetManifestRef().Clone()
	}
	return &manifestCopyAccounting{
		manifestRef:         manifestRef,
		sourceBucketID:      sourceBucketID,
		destinationBucketID: destinationBucketID,
		sourceIdentity:      sourceIdentity,
		destinationIdentity: manifestCopyIdentityLocal,
		counters:            &manifestCopyCounters{},
	}
}

func (a *manifestCopyAccounting) withBuckets(sourceBucketID, destinationBucketID string) *manifestCopyAccounting {
	if a == nil {
		return nil
	}
	updated := &manifestCopyAccounting{
		manifestRef:         a.manifestRef,
		sourceBucketID:      sourceBucketID,
		destinationBucketID: destinationBucketID,
		sourceIdentity:      manifestCopyIdentityExternal,
		destinationIdentity: manifestCopyIdentityLocal,
		counters:            a.counters,
	}
	if sourceBucketID == "" || (destinationBucketID != "" && sourceBucketID == destinationBucketID) {
		updated.sourceIdentity = manifestCopyIdentityLocal
	}
	return updated
}

func (a *manifestCopyAccounting) apply(stats bucket_lookup.ObjectCopyStats) bucket_lookup.ObjectCopyStats {
	if a == nil || a.counters == nil {
		return stats
	}
	readCount, readBytes := a.counters.snapshot()
	stats.DemandReadCount = int64(readCount)
	stats.DemandReadBytes = int64(readBytes)
	return stats
}

func (a *manifestCopyAccounting) sameCandidate(manifestSnapshot *bldr_manifest.ManifestSnapshot) bool {
	if a == nil || a.manifestRef == nil || manifestSnapshot == nil {
		return false
	}
	return bldr_manifest_world.ManifestObjectRefsSameExecutable(a.manifestRef, manifestSnapshot.GetManifestRef()) && a.manifestRef.EqualVT(manifestSnapshot.GetManifestRef())
}

type manifestDemandObservation struct {
	accounting *manifestCopyAccounting
	counter    *block.ReadCounter
}

func (o *manifestDemandObservation) register() {
	if o == nil || o.accounting == nil || o.counter == nil || o.accounting.counters == nil {
		return
	}
	o.accounting.counters.register(o.counter)
}

func (o *manifestDemandObservation) snapshot() block.ReadCounterSnapshot {
	if o == nil || o.counter == nil {
		return block.ReadCounterSnapshot{}
	}
	snapshot := o.counter.Snapshot()
	if o.accounting != nil && o.accounting.counters != nil {
		o.accounting.counters.observe(o.counter)
	}
	return snapshot
}

func (o *manifestDemandObservation) finish() {
	if o == nil || o.accounting == nil || o.counter == nil || o.accounting.counters == nil {
		return
	}
	o.accounting.counters.finish(o.counter)
}

func (t *pluginInstance) setManifestCopySelection(
	ctx context.Context,
	manifestSnapshot *bldr_manifest.ManifestSnapshot,
	sourceBucketID, destinationBucketID string,
) *manifestCopyAccounting {
	if manifestSnapshot == nil || manifestSnapshot.GetManifestRef() == nil {
		t.manifestCopyAccounting.Store(nil)
		return nil
	}
	for {
		current := t.manifestCopyAccounting.Load()
		if current != nil && current.sameCandidate(manifestSnapshot) {
			if current.sourceBucketID == sourceBucketID && current.destinationBucketID == destinationBucketID {
				return current
			}
			updated := current.withBuckets(sourceBucketID, destinationBucketID)
			if t.manifestCopyAccounting.CompareAndSwap(current, updated) {
				return updated
			}
			continue
		}
		next := newManifestCopyAccounting(manifestSnapshot, sourceBucketID, destinationBucketID)
		if t.manifestCopyAccounting.CompareAndSwap(current, next) {
			trace.Log(ctx, "manifest-copy-selection", "selected")
			trace.Log(ctx, "manifest-copy-source-bucket", sourceBucketID)
			trace.Log(ctx, "manifest-copy-destination-bucket", destinationBucketID)
			trace.Log(ctx, "manifest-copy-source-identity", string(next.sourceIdentity))
			trace.Log(ctx, "manifest-copy-destination-identity", string(next.destinationIdentity))
			t.setManifestCopyStatus(manifestCopyPhaseSelected, manifestCopyClassImmediate, manifestSnapshot, next, bucket_lookup.ObjectCopyStats{})
			t.emitManifestCopyStartupMark(manifestCopyPhaseSelected, bucket_lookup.ObjectCopyStats{}, next)
			return next
		}
	}
}

func (t *pluginInstance) updateManifestCopyBuckets(
	ctx context.Context,
	expected *manifestCopyAccounting,
	sourceBucketID, destinationBucketID string,
) *manifestCopyAccounting {
	if expected == nil {
		return nil
	}
	updated := expected.withBuckets(sourceBucketID, destinationBucketID)
	if updated == nil || !t.manifestCopyAccounting.CompareAndSwap(expected, updated) {
		return expected
	}
	trace.Log(ctx, "manifest-copy-source-bucket", sourceBucketID)
	trace.Log(ctx, "manifest-copy-destination-bucket", destinationBucketID)
	trace.Log(ctx, "manifest-copy-source-identity", string(updated.sourceIdentity))
	trace.Log(ctx, "manifest-copy-destination-identity", string(updated.destinationIdentity))
	return updated
}

func (t *pluginInstance) manifestCopyAccountingForExecution(
	ctx context.Context,
	manifestSnapshot *bldr_manifest.ManifestSnapshot,
) *manifestCopyAccounting {
	if accounting := t.manifestCopyAccounting.Load(); accounting != nil && accounting.sameCandidate(manifestSnapshot) {
		return accounting
	}
	if manifestSnapshot == nil || manifestSnapshot.GetManifestRef() == nil {
		return nil
	}
	return t.setManifestCopySelection(
		ctx,
		manifestSnapshot,
		manifestSnapshot.GetManifestRef().GetBucketId(),
		"",
	)
}
