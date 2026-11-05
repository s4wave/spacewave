package provider_local

import (
	"context"

	provider_gccleanup "github.com/s4wave/spacewave/core/provider/gccleanup"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume"
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

// collectGCRootlessBlocks runs one collection pass of the account's volume,
// which frees what no held named root reaches.
func (a *ProviderAccount) collectGCRootlessBlocks(ctx context.Context) (block_gc.CollectResult, error) {
	// Use the installed collector when a test replaces the volume pass.
	if a.gcCleanupCollect != nil {
		return a.gcCleanupCollect(ctx)
	}

	// Refuse to collect while the block stores write to an SQL primary, whose
	// blocks reference volume blocks without child lists.
	if a.sqlPrimary.Load() {
		a.le.Warn("skipping gc cleanup while block stores write to an SQL primary")
		return block_gc.CollectResult{}, nil
	}

	// Collect through the volume when it collects.
	collector, ok := a.vol.(volume.Collector)
	if !ok {
		return block_gc.CollectResult{}, nil
	}
	return collector.Collect(ctx)
}

// removeSharedObjectGCRefs releases every named root of a deleted shared
// object's bucket, so the collector frees what only they reached.
func (a *ProviderAccount) removeSharedObjectGCRefs(
	ctx context.Context,
	_ string,
	bucketID string,
	le *logrus.Entry,
) {
	releaser, ok := a.vol.(volume.BucketReleaser)
	if !ok {
		return
	}
	if err := releaser.ReleaseBucket(ctx, bucketID); err != nil {
		le.WithError(err).Warn("failed to release the deleted shared object's roots")
	}
}
