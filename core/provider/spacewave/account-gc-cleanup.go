package provider_spacewave

import (
	"context"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

func (a *ProviderAccount) triggerGCCleanup() {
	a.gcCleanupRunner.Trigger()
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
