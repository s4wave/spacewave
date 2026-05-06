package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

func (a *ProviderAccount) triggerGCCleanup() {
	a.gcCleanupBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.gcCleanupGeneration++
		broadcast()
	})
}

func (a *ProviderAccount) runGCCleanup(ctx context.Context) error {
	var completed uint64
	for {
		var (
			generation uint64
			waitCh     <-chan struct{}
		)
		a.gcCleanupBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if a.gcCleanupGeneration > completed {
				generation = a.gcCleanupGeneration
				return
			}
			waitCh = getWaitCh()
		})
		if generation == 0 {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-waitCh:
				continue
			}
		}

		stats, err := a.collectGCRootlessBlocks(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			return err
		}
		logGCCleanupStats(
			a.le.WithField("component", "gc-cleanup-runner"),
			"GC swept nodes after provider account cleanup",
			stats,
		)
		completed = generation
	}
}

func (a *ProviderAccount) collectGCRootlessBlocks(ctx context.Context) (*block_gc.Stats, error) {
	if a.gcCleanupCollect != nil {
		return a.gcCleanupCollect(ctx)
	}
	kvVol, ok := a.vol.(kvtx_volume.KvtxVolume)
	if !ok {
		return nil, nil
	}
	rg := kvVol.GetRefGraph()
	if rg == nil {
		return nil, nil
	}
	return block_gc.NewCollector(rg, a.vol, nil).Collect(ctx)
}

func (a *ProviderAccount) removeSharedObjectGCRefs(
	ctx context.Context,
	id string,
	le *logrus.Entry,
) {
	kvVol, ok := a.vol.(kvtx_volume.KvtxVolume)
	if !ok {
		return
	}
	rg := kvVol.GetRefGraph()
	if rg == nil {
		return
	}
	bstoreID := SobjectBlockStoreID(id)
	bucketID := BlockStoreBucketID(a.accountID, bstoreID)
	bucketIRI := block_gc.BucketIRI(bucketID)
	providerID := a.p.info.GetProviderId()
	gcOps := block_gc.NewGCStoreOps(a.vol, rg)
	if err := gcOps.RemoveGCRef(ctx,
		block_gc.NodeGCRoot,
		bucketIRI,
	); err != nil {
		le.WithError(err).Warn("failed to remove GC root ref")
	}
	if err := gcOps.RemoveGCRef(ctx,
		ProviderIRI(providerID),
		bucketIRI,
	); err != nil {
		le.WithError(err).Warn("failed to remove GC ref")
	}
}

func (a *ProviderAccount) removeProviderAccountGCRef(
	ctx context.Context,
	le *logrus.Entry,
) {
	kvVol, ok := a.vol.(kvtx_volume.KvtxVolume)
	if !ok {
		return
	}
	rg := kvVol.GetRefGraph()
	if rg == nil {
		return
	}
	providerID := a.p.info.GetProviderId()
	gcOps := block_gc.NewGCStoreOps(a.vol, rg)
	if err := gcOps.RemoveGCRef(ctx,
		block_gc.NodeGCRoot,
		ProviderIRI(providerID),
	); err != nil {
		le.WithError(err).Warn("GC: failed to remove provider edge")
	}
}

func logGCCleanupStats(le *logrus.Entry, msg string, stats *block_gc.Stats) {
	if stats == nil || stats.NodesSwept == 0 {
		return
	}
	le.WithField("nodes-swept", stats.NodesSwept).
		WithField("duration", stats.Duration.String()).
		WithField("unreferenced-nodes", stats.UnreferencedNodeCount).
		WithField("remove-node-refs", stats.RemoveNodeRefsCount).
		WithField("remove-unreferenced-edges", stats.RemoveUnreferencedEdgeCount).
		WithField("on-swept-callbacks", stats.OnSweptCount).
		WithField("remove-blocks", stats.RemoveBlockCount).
		WithField("unreferenced-scan-duration", stats.UnreferencedScanDuration.String()).
		WithField("remove-node-refs-duration", stats.RemoveNodeRefsDuration.String()).
		WithField("remove-unreferenced-edge-duration", stats.RemoveUnreferencedEdgeDuration.String()).
		WithField("on-swept-duration", stats.OnSweptDuration.String()).
		WithField("remove-block-duration", stats.RemoveBlockDuration.String()).
		Info(msg)
}
