package plugin_host_scheduler

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bucket "github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

// manifestCopyClass is the startup-copy scheduling decision for a
// manifest: immediate, deferred to a ready gate, or suppressed.
type manifestCopyClass string

const (
	manifestCopyClassImmediate              manifestCopyClass = "immediate"
	manifestCopyClassAfterExecuteReady      manifestCopyClass = "after-execute-ready"
	manifestCopyClassAfterStartupGroupReady manifestCopyClass = "after-startup-group-ready"
	manifestCopyClassSuppressed             manifestCopyClass = "suppressed"
)

// manifestCopyPhase is the lifecycle phase of one manifest copy.
type manifestCopyPhase string

const (
	manifestCopyPhaseSelected               manifestCopyPhase = "selected"
	manifestCopyPhaseWaiting                manifestCopyPhase = "waiting-for-running"
	manifestCopyPhaseWaitingForStartupGroup manifestCopyPhase = "waiting-for-startup-group"
	manifestCopyPhaseWaitingForAdmission    manifestCopyPhase = "waiting-for-admission"
	manifestCopyPhaseCopying                manifestCopyPhase = "copying"
	manifestCopyPhaseDone                   manifestCopyPhase = "done"
	manifestCopyPhaseFailed                 manifestCopyPhase = "failed"
	manifestCopyPhaseSuppressed             manifestCopyPhase = "suppressed"
)

// manifestCopyStatus tracks the copy state and identities for one
// manifest.
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

// classifyManifestCopy decides the startup-copy class for a manifest
// snapshot.
func (t *pluginInstance) classifyManifestCopy(manifestSnapshot *bldr_manifest.ManifestSnapshot) manifestCopyClass {
	if t == nil || manifestSnapshot == nil {
		return manifestCopyClassImmediate
	}
	if t.c != nil && t.c.conf != nil {
		ref := manifestSnapshot.GetManifestRef()
		if ref != nil &&
			ref.GetBucketId() != "" &&
			slices.Contains(t.c.conf.GetNoCopyBucketIds(), ref.GetBucketId()) {
			return manifestCopyClassSuppressed
		}
	}
	gate := t.c.getManifestCopyGate()
	if gate != nil && !gate.IsReady() {
		return manifestCopyClassAfterStartupGroupReady
	}
	if t.executePluginRoutine == nil || t.runningPluginCtr == nil {
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

// setManifestCopyStatus stores a new status for the manifest copy.
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

// setDownloadManifestState updates the download routine's state.
func (t *pluginInstance) setDownloadManifestState(
	ctx context.Context,
	manifestSnapshot *bldr_manifest.ManifestSnapshot,
	destinationBucketID string,
) bool {
	if t.c.conf.GetDisableCopyManifest() {
		return false
	}
	if t.classifyManifestCopy(manifestSnapshot) != manifestCopyClassSuppressed {
		_, changed, _, _ := t.downloadManifestRoutine.SetState(manifestSnapshot)
		return changed
	}

	_, changed, _, _ := t.downloadManifestRoutine.SetState(nil)
	ref := manifestSnapshot.GetManifestRef()
	accounting := t.setManifestCopySelection(
		ctx,
		manifestSnapshot,
		ref.GetBucketId(),
		destinationBucketID,
	)
	trace.Log(ctx, "manifest-copy-phase", string(manifestCopyPhaseSuppressed))
	trace.Log(ctx, "manifest-copy-class", string(manifestCopyClassSuppressed))
	t.setManifestCopyStatus(
		manifestCopyPhaseSuppressed,
		manifestCopyClassSuppressed,
		manifestSnapshot,
		accounting,
		bucket_lookup.ObjectCopyStats{},
	)
	t.emitManifestCopyStartupMark(manifestCopyPhaseSuppressed, bucket_lookup.ObjectCopyStats{}, accounting)
	return changed
}

// waitForManifestCopyReady waits until the manifest copy is ready to use.
func (t *pluginInstance) waitForManifestCopyReady(
	ctx context.Context,
	class manifestCopyClass,
	manifestSnapshot *bldr_manifest.ManifestSnapshot,
	accounting *manifestCopyAccounting,
) error {
	switch class {
	case manifestCopyClassImmediate, manifestCopyClassSuppressed:
		return nil
	case manifestCopyClassAfterExecuteReady:
		ctx, task := trace.NewTask(ctx, "bldr/plugin-host-scheduler/download-manifest/wait-for-startup-execute-ready")
		defer task.End()
		t.logPluginAccountingFields(ctx)
		logManifestSnapshotAccountingFields(ctx, "download", manifestSnapshot)

		t.setManifestCopyStatus(manifestCopyPhaseWaiting, class, manifestSnapshot, accounting, bucket_lookup.ObjectCopyStats{})
		t.emitManifestCopyStartupMark(manifestCopyPhaseWaiting, bucket_lookup.ObjectCopyStats{}, accounting)
		_, err := t.runningPluginCtr.WaitValue(ctx, nil)
		return err
	case manifestCopyClassAfterStartupGroupReady:
		ctx, task := trace.NewTask(ctx, "bldr/plugin-host-scheduler/download-manifest/wait-for-startup-group-ready")
		defer task.End()
		t.logPluginAccountingFields(ctx)
		logManifestSnapshotAccountingFields(ctx, "download", manifestSnapshot)

		t.setManifestCopyStatus(manifestCopyPhaseWaitingForStartupGroup, class, manifestSnapshot, accounting, bucket_lookup.ObjectCopyStats{})
		t.emitManifestCopyStartupMark(manifestCopyPhaseWaitingForStartupGroup, bucket_lookup.ObjectCopyStats{}, accounting)
		gate := t.c.getManifestCopyGate()
		if gate == nil {
			return nil
		}
		return gate.WaitReady(ctx)
	default:
		return errors.Errorf("unknown manifest copy class: %q", class)
	}
}

// checkDownloadManifestCurrent verifies the context is still live and the
// manifest is still the download routine's selected request, returning
// context.Canceled for a replaced request. The routine is nil in
// direct-executor tests that bypass selection; the equality check applies
// only when the routine exists.
func (t *pluginInstance) checkDownloadManifestCurrent(
	ctx context.Context,
	manifestSnapshot *bldr_manifest.ManifestSnapshot,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.downloadManifestRoutine == nil {
		return nil
	}
	if selected := t.downloadManifestRoutine.GetState(); selected == nil || !selected.EqualVT(manifestSnapshot) {
		return context.Canceled
	}
	return nil
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

	if t.c.conf.GetDisableCopyManifest() ||
		manifestSnapshot == nil ||
		manifestSnapshot.GetManifestRef() == nil ||
		t.classifyManifestCopy(manifestSnapshot) == manifestCopyClassSuppressed {
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

	if err := t.waitForManifestCopyReady(ctx, class, manifestSnapshot, accounting); err != nil {
		return err
	}

	// Acquire the aggregate manifest copy allowance. The permit is held
	// through traversal, local-ref publication, and Sync so the configured
	// bound covers the copy's total active footprint. The copying phase
	// starts only after admission.
	t.setManifestCopyStatus(manifestCopyPhaseWaitingForAdmission, class, manifestSnapshot, accounting, bucket_lookup.ObjectCopyStats{})
	t.emitManifestCopyStartupMark(manifestCopyPhaseWaitingForAdmission, bucket_lookup.ObjectCopyStats{}, accounting)
	releaseManifestCopy, err := t.c.manifestCopyMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer releaseManifestCopy()

	// Recheck after admission: another request may have replaced this one
	// while the allowance was queued.
	if err := t.checkDownloadManifestCurrent(ctx, manifestSnapshot); err != nil {
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

	// Build cursors outside AccessWorldState so source reads and destination
	// block writes cannot wait while holding a world-state access.
	dest, err := ws.BuildStorageCursor(ctx)
	if err != nil {
		return err
	}
	defer dest.Release()
	src, err := bldr_manifest_world.FollowObjectRefReadOnly(ctx, dest, ref)
	if err != nil {
		return err
	}
	defer src.Release()
	destBucketID := dest.GetOpArgs().GetBucketId()
	accounting = t.updateManifestCopyBuckets(ctx, accounting, src.GetOpArgs().GetBucketId(), destBucketID)
	trace.Log(ctx, "world-bucket-id", destBucketID)
	trace.Log(ctx, "source-bucket-id", src.GetOpArgs().GetBucketId())
	le.Infof("copying manifest DAG from bucket %s to %s", src.GetOpArgs().GetBucketId(), destBucketID)

	// Copy the manifest DAG: through the materializer plugin when configured,
	// otherwise with the in-process copy engine. A failed RPC must not fall
	// back to native copying; the scheduler routine owns retry.
	copyCtx, copyTask := trace.NewTask(ctx, "bldr/plugin-host-scheduler/download-manifest/copy-dag")
	trace.Log(copyCtx, "accounting-phase", "decode-verify-deserialize-block-publish")
	var localRef *bucket.ObjectRef
	var stats bucket_lookup.ObjectCopyStats
	if pluginID := t.c.conf.GetMaterializerPluginId(); pluginID != "" {
		localRef, stats, err = t.c.materializeManifest(copyCtx, pluginID, dest, src, t.c.conf.manifestCopyConcurrency())
	} else {
		localRef, stats, err = bucket_lookup.CopyObjectToBucketWithStats(
			copyCtx,
			dest,
			src,
			bldr_manifest.NewManifestBlock,
			t.c.conf.manifestCopyConcurrency(),
			false,
			nil,
		)
	}
	copyStats = accounting.apply(stats)
	copyTask.End()
	if err != nil {
		return errors.Wrap(err, "copy manifest block DAG")
	}
	logObjectRefAccountingFields(ctx, "local-manifest", localRef)

	// The request may have been replaced while the copy ran; publication
	// applies only to the currently selected manifest.
	if err := t.checkDownloadManifestCurrent(ctx, manifestSnapshot); err != nil {
		return err
	}

	if !t.c.conf.GetDisableStoreManifest() {
		storeCtx, storeTask := trace.NewTask(ctx, "bldr/plugin-host-scheduler/download-manifest/store-local-ref")
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

	syncCtx, syncTask := trace.NewTask(ctx, "bldr/plugin-host-scheduler/download-manifest/sync")
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
}
