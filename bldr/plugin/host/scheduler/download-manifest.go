package plugin_host_scheduler

import (
	"context"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

type manifestCopyClass string

const (
	manifestCopyClassImmediate         manifestCopyClass = "immediate"
	manifestCopyClassAfterExecuteReady manifestCopyClass = "after-execute-ready"
)

type manifestCopyPhase string

const (
	manifestCopyPhaseSelected manifestCopyPhase = "selected"
	manifestCopyPhaseWaiting  manifestCopyPhase = "waiting-for-running"
	manifestCopyPhaseCopying  manifestCopyPhase = "copying"
	manifestCopyPhaseDone     manifestCopyPhase = "done"
	manifestCopyPhaseFailed   manifestCopyPhase = "failed"
)

type manifestCopyStatus struct {
	phase               manifestCopyPhase
	class               manifestCopyClass
	manifestRef         string
	sourceBucketID      string
	destinationBucketID string
	sourceIdentity      manifestCopyIdentity
	destinationIdentity manifestCopyIdentity
	stats               bucket_lookup.ObjectCopyStats
}

func (t *pluginInstance) classifyManifestCopy(manifestSnapshot *bldr_manifest.ManifestSnapshot) manifestCopyClass {
	if t == nil || t.executePluginRoutine == nil || t.runningPluginCtr == nil || manifestSnapshot == nil {
		return manifestCopyClassImmediate
	}
	execState := t.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		return manifestCopyClassImmediate
	}
	if !bldr_manifest_world.ManifestObjectRefsSameExecutable(execState.manifestSnapshot.GetManifestRef(), manifestSnapshot.GetManifestRef()) {
		return manifestCopyClassImmediate
	}
	if t.runningPluginCtr.GetValue() != nil {
		return manifestCopyClassImmediate
	}
	return manifestCopyClassAfterExecuteReady
}

func (t *pluginInstance) setManifestCopyStatus(
	phase manifestCopyPhase,
	class manifestCopyClass,
	manifestSnapshot *bldr_manifest.ManifestSnapshot,
	accounting *manifestCopyAccounting,
	stats bucket_lookup.ObjectCopyStats,
) {
	if t == nil ||
		t.manifestCopyStatus == nil ||
		accounting == nil ||
		t.manifestCopyAccounting.Load() != accounting {
		return
	}
	var ref string
	if manifestSnapshot != nil && manifestSnapshot.GetManifestRef() != nil {
		ref = manifestSnapshot.GetManifestRef().MarshalString()
	}
	status := &manifestCopyStatus{
		phase:               phase,
		class:               class,
		manifestRef:         ref,
		stats:               accounting.apply(stats),
		sourceBucketID:      accounting.sourceBucketID,
		destinationBucketID: accounting.destinationBucketID,
		sourceIdentity:      accounting.sourceIdentity,
		destinationIdentity: accounting.destinationIdentity,
	}
	t.manifestCopyStatus.SetValue(status)
}

func (t *pluginInstance) waitForStartupExecuteReady(
	ctx context.Context,
	class manifestCopyClass,
	manifestSnapshot *bldr_manifest.ManifestSnapshot,
	accounting *manifestCopyAccounting,
) error {
	if class != manifestCopyClassAfterExecuteReady {
		return nil
	}
	ctx, task := trace.NewTask(ctx, "bldr/plugin-host-scheduler/download-manifest/wait-for-startup-execute-ready")
	defer task.End()
	t.logPluginAccountingFields(ctx)
	logManifestSnapshotAccountingFields(ctx, "download", manifestSnapshot)

	t.setManifestCopyStatus(manifestCopyPhaseWaiting, class, manifestSnapshot, accounting, bucket_lookup.ObjectCopyStats{})
	t.emitManifestCopyStartupMark(manifestCopyPhaseWaiting, bucket_lookup.ObjectCopyStats{}, accounting)
	_, err := t.runningPluginCtr.WaitValue(ctx, nil)
	return err
}

// execDownloadManifest copies manifest blocks from the source bucket to the world bucket.
func (t *pluginInstance) execDownloadManifest(
	ctx context.Context,
	manifestSnapshot *bldr_manifest.ManifestSnapshot,
) (rerr error) {
	defer func() {
		if rerr != nil {
			trace.Log(ctx, "manifest-copy-phase", "error")
		}
		t.c.recordPluginStatusError(t.pluginID, t.instanceKey, "download plugin manifest", rerr)
	}()

	if t.c.conf.GetDisableCopyManifest() || manifestSnapshot == nil || manifestSnapshot.GetManifestRef() == nil {
		return nil
	}

	ctx, task := trace.NewTask(ctx, "bldr/plugin-host-scheduler/download-manifest")
	defer task.End()
	le := t.le
	ref := manifestSnapshot.GetManifestRef()
	blockRef := ref.GetRootRef()
	if blockRef.GetEmpty() {
		return errors.New("manifest ref has empty root block ref")
	}
	var manifestMeta *bldr_manifest.ManifestMeta
	if !t.c.conf.GetDisableStoreManifest() {
		manifestMeta = manifestSnapshot.GetManifest().GetMeta()
		if err := manifestMeta.Validate(false); err != nil {
			return errors.Wrap(err, "manifest snapshot metadata")
		}
	}
	class := t.classifyManifestCopy(manifestSnapshot)
	accounting := t.manifestCopyAccountingForExecution(ctx, manifestSnapshot)
	trace.Log(ctx, "manifest-copy-phase", "selection")
	var copyStats bucket_lookup.ObjectCopyStats
	defer func() {
		if rerr != nil {
			copyStats = accounting.apply(copyStats)
			t.setManifestCopyStatus(manifestCopyPhaseFailed, class, manifestSnapshot, accounting, copyStats)
			t.emitManifestCopyStartupMark(manifestCopyPhaseFailed, copyStats, accounting)
		}
	}()
	t.logPluginAccountingFields(ctx)
	trace.Log(ctx, "startup-fetch-kind", "background-manifest-dag-copy")
	trace.Log(ctx, "manifest-copy-class", string(class))

	if err := t.waitForStartupExecuteReady(ctx, class, manifestSnapshot, accounting); err != nil {
		return err
	}
	materializerCtx, materializerTask := trace.NewTask(ctx, "bldr/plugin-host-scheduler/materializer")
	trace.Log(materializerCtx, "materializer-phase", "start")
	defer func() {
		trace.Log(materializerCtx, "materializer-phase", "end")
		materializerTask.End()
	}()
	t.setManifestCopyStatus(manifestCopyPhaseCopying, class, manifestSnapshot, accounting, bucket_lookup.ObjectCopyStats{})
	t.emitManifestCopyStartupMark(manifestCopyPhaseCopying, bucket_lookup.ObjectCopyStats{}, accounting)
	ws, err := t.c.worldStateCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// Access the world root bucket (dest) then the manifest source bucket (src).
	accessCtx, accessTask := trace.NewTask(ctx, "bldr/plugin-host-scheduler/download-manifest/access-world-state")
	err = ws.AccessWorldState(accessCtx, nil, func(dest *bucket_lookup.Cursor) error {
		src, err := bldr_manifest_world.FollowObjectRefReadOnly(accessCtx, dest, ref)
		if err != nil {
			return err
		}
		defer src.Release()
		destBucketID := dest.GetOpArgs().GetBucketId()
		accounting = t.updateManifestCopyBuckets(accessCtx, accounting, src.GetOpArgs().GetBucketId(), destBucketID)
		trace.Log(accessCtx, "world-bucket-id", destBucketID)
		trace.Log(accessCtx, "source-bucket-id", src.GetOpArgs().GetBucketId())
		le.Infof("copying manifest DAG from bucket %s to %s", src.GetOpArgs().GetBucketId(), destBucketID)

		copyCtx, copyTask := trace.NewTask(accessCtx, "bldr/plugin-host-scheduler/download-manifest/copy-dag")
		trace.Log(copyCtx, "accounting-phase", "decode-verify-deserialize-block-publish")
		localRef, stats, err := bucket_lookup.CopyObjectToBucketWithStats(
			copyCtx,
			dest,
			src,
			bldr_manifest.NewManifestBlock,
			1,
			false,
			nil,
		)
		copyStats = accounting.apply(stats)
		copyTask.End()
		if err != nil {
			return errors.Wrap(err, "copy manifest block DAG")
		}
		logObjectRefAccountingFields(accessCtx, "local-manifest", localRef)

		if !t.c.conf.GetDisableStoreManifest() {
			storeCtx, storeTask := trace.NewTask(accessCtx, "bldr/plugin-host-scheduler/download-manifest/store-local-ref")
			trace.Log(storeCtx, "accounting-phase", "world-op-store-local-manifest-ref")
			trace.Log(storeCtx, "manifest-copy-phase", "local-ref-publication")
			manifestKey := bldr_manifest.NewManifestKey(t.c.objKey, manifestMeta)
			if err := bldr_manifest_world.ExStoreManifestOp(
				storeCtx,
				ws,
				t.c.peerID,
				manifestKey,
				[]string{t.c.objKey},
				bldr_manifest.NewManifestRef(manifestMeta, localRef),
			); err != nil {
				storeTask.End()
				return errors.Wrap(err, "store local manifest ref")
			}
			storeTask.End()
		}

		syncCtx, syncTask := trace.NewTask(accessCtx, "bldr/plugin-host-scheduler/download-manifest/sync")
		trace.Log(syncCtx, "accounting-phase", "world-sync-block-barrier-and-head-commit")
		synced, syncErr := ws.Sync(syncCtx)
		if syncErr != nil {
			syncTask.End()
			return errors.Wrap(syncErr, "sync local manifest blocks")
		}
		if synced && dest.GetTransformer() == nil {
			copyStats.DestinationDurableBytes = copyStats.LogicalSourceBytes
			copyStats.DestinationDurableBytesKnown = true
		}
		trace.Logf(syncCtx, "destination-durable-bytes", "%d", copyStats.DestinationDurableBytes)
		trace.Logf(syncCtx, "destination-durable-bytes-known", "%t", copyStats.DestinationDurableBytesKnown)
		trace.Log(syncCtx, "manifest-copy-phase", "sync-complete")
		syncTask.End()
		copyStats = accounting.apply(copyStats)
		t.setManifestCopyStatus(manifestCopyPhaseDone, class, manifestSnapshot, accounting, copyStats)
		t.emitManifestCopyStartupMark(manifestCopyPhaseDone, copyStats, accounting)
		le.Info("manifest download complete")
		return nil
	})
	accessTask.End()
	return err
}
