package provider_local

import (
	"context"

	provider_gccleanup "github.com/s4wave/spacewave/core/provider/gccleanup"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

func (a *ProviderAccount) newGCCleanupRunner() *provider_gccleanup.Runner {
	return provider_gccleanup.NewRunner(
		a.le.WithField("component", "gc-cleanup-runner"),
		"GC swept nodes after local provider account cleanup",
		a.collectGCRootlessBlocks,
	)
}

func (a *ProviderAccount) triggerGCCleanup() {
	a.gcCleanupRunner.Trigger()
}

// WaitGCCleanup waits for pending account GC cleanup to finish.
func (a *ProviderAccount) WaitGCCleanup(ctx context.Context) error {
	return a.gcCleanupRunner.Wait(ctx)
}

func (a *ProviderAccount) runGCCleanup(ctx context.Context) error {
	return a.gcCleanupRunner.Run(ctx)
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
	providerID string,
	bucketID string,
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
	gcOps := block_gc.NewGCStoreOps(a.vol, rg)
	bucketIRI := block_gc.BucketIRI(bucketID)
	if err := gcOps.RemoveGCRef(ctx, block_gc.NodeGCRoot, bucketIRI); err != nil {
		le.WithError(err).Warn("failed to remove gc root ref for deleted sobject bucket")
	}
	if err := gcOps.RemoveGCRef(ctx, ProviderIRI(providerID), bucketIRI); err != nil {
		le.WithError(err).Warn("failed to remove gc ref for deleted sobject")
	}
}
