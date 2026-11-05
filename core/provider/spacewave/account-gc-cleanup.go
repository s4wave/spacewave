package provider_spacewave

import (
	"context"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

// triggerGCCleanup asks the cleanup runner for a collection pass.
func (a *ProviderAccount) triggerGCCleanup() {
	a.gcCleanupRunner.Trigger()
}

// runGCCleanup runs the cleanup runner until ctx ends.
func (a *ProviderAccount) runGCCleanup(ctx context.Context) error {
	return a.gcCleanupRunner.Run(ctx)
}

// collectGCRootlessBlocks runs one collection pass of the account's volume,
// which frees what no held named root reaches.
func (a *ProviderAccount) collectGCRootlessBlocks(ctx context.Context) (block_gc.CollectResult, error) {
	if a.gcCleanupCollect != nil {
		return a.gcCleanupCollect(ctx)
	}
	collector, ok := a.vol.(volume.Collector)
	if !ok {
		return block_gc.CollectResult{}, nil
	}
	return collector.Collect(ctx)
}

// removeSharedObjectGCRefs releases every named root of a deleted shared
// object's block store bucket, so the collector frees what only they reached.
func (a *ProviderAccount) removeSharedObjectGCRefs(
	ctx context.Context,
	id string,
	le *logrus.Entry,
) {
	releaser, ok := a.vol.(volume.BucketReleaser)
	if !ok {
		return
	}
	bucketID := BlockStoreBucketID(a.accountID, SobjectBlockStoreID(id))
	if err := releaser.ReleaseBucket(ctx, bucketID); err != nil {
		le.WithError(err).Warn("failed to release the deleted shared object's roots")
	}
}

// releaseAccountRoots releases every named root of the deleted account's
// buckets, so the collector frees what only they reached.
func (a *ProviderAccount) releaseAccountRoots(ctx context.Context, le *logrus.Entry) {
	releaser, ok := a.vol.(volume.BucketReleaser)
	if !ok {
		return
	}
	if err := releaser.ReleaseBucketPrefix(ctx, ProviderIDPrefix+a.accountID+"/"); err != nil {
		le.WithError(err).Warn("failed to release the deleted account's roots")
	}
}
