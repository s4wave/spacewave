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
	manifestCopyPhaseWaiting manifestCopyPhase = "waiting-for-running"
	manifestCopyPhaseCopying manifestCopyPhase = "copying"
	manifestCopyPhaseDone    manifestCopyPhase = "done"
	manifestCopyPhaseFailed  manifestCopyPhase = "failed"
)

type manifestCopyStatus struct {
	phase       manifestCopyPhase
	class       manifestCopyClass
	manifestRef string
}

func (t *pluginInstance) classifyManifestCopy(manifestSnapshot *bldr_manifest.ManifestSnapshot) manifestCopyClass {
	if t == nil || t.executePluginRoutine == nil || t.runningPluginCtr == nil || manifestSnapshot == nil {
		return manifestCopyClassImmediate
	}
	execState := t.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		return manifestCopyClassImmediate
	}
	if !manifestObjectRefsSameExecutable(execState.manifestSnapshot.GetManifestRef(), manifestSnapshot.GetManifestRef()) {
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
) {
	if t == nil || t.manifestCopyStatus == nil {
		return
	}
	var ref string
	if manifestSnapshot != nil && manifestSnapshot.GetManifestRef() != nil {
		ref = manifestSnapshot.GetManifestRef().MarshalString()
	}
	t.manifestCopyStatus.SetValue(&manifestCopyStatus{
		phase:       phase,
		class:       class,
		manifestRef: ref,
	})
}

func (t *pluginInstance) waitForStartupExecuteReady(
	ctx context.Context,
	class manifestCopyClass,
	manifestSnapshot *bldr_manifest.ManifestSnapshot,
) error {
	if class != manifestCopyClassAfterExecuteReady {
		return nil
	}
	ctx, task := trace.NewTask(ctx, "bldr/plugin-host-scheduler/download-manifest/wait-for-startup-execute-ready")
	defer task.End()
	t.logPluginAccountingFields(ctx)
	logManifestSnapshotAccountingFields(ctx, "download", manifestSnapshot)

	t.setManifestCopyStatus(manifestCopyPhaseWaiting, class, manifestSnapshot)
	_, err := t.runningPluginCtr.WaitValue(ctx, nil)
	return err
}

// execDownloadManifest copies manifest blocks from the source bucket to the world bucket.
func (t *pluginInstance) execDownloadManifest(
	ctx context.Context,
	manifestSnapshot *bldr_manifest.ManifestSnapshot,
) (rerr error) {
	defer func() {
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
	defer func() {
		if rerr != nil {
			t.setManifestCopyStatus(manifestCopyPhaseFailed, class, manifestSnapshot)
		}
	}()
	t.logPluginAccountingFields(ctx)
	logManifestSnapshotAccountingFields(ctx, "download", manifestSnapshot)
	trace.Log(ctx, "startup-fetch-kind", "background-manifest-dag-copy")
	trace.Log(ctx, "manifest-copy-class", string(class))

	if err := t.waitForStartupExecuteReady(ctx, class, manifestSnapshot); err != nil {
		return err
	}
	t.setManifestCopyStatus(manifestCopyPhaseCopying, class, manifestSnapshot)

	ws, err := t.c.worldStateCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// Access the world root bucket (dest) then the manifest source bucket (src).
	accessCtx, accessTask := trace.NewTask(ctx, "bldr/plugin-host-scheduler/download-manifest/access-world-state")
	err = ws.AccessWorldState(accessCtx, nil, func(dest *bucket_lookup.Cursor) error {
		return ws.AccessWorldState(accessCtx, ref, func(src *bucket_lookup.Cursor) error {
			destBucketID := dest.GetOpArgs().GetBucketId()
			trace.Log(accessCtx, "world-bucket-id", destBucketID)
			trace.Log(accessCtx, "source-bucket-id", src.GetOpArgs().GetBucketId())
			le.Infof("copying manifest DAG from bucket %s to %s", src.GetOpArgs().GetBucketId(), destBucketID)

			copyCtx, copyTask := trace.NewTask(accessCtx, "bldr/plugin-host-scheduler/download-manifest/copy-dag")
			trace.Log(copyCtx, "accounting-phase", "decode-verify-deserialize-block-publish")
			localRef, err := bucket_lookup.CopyObjectToBucket(
				copyCtx,
				dest,
				src,
				bldr_manifest.NewManifestBlock,
				1,
				false,
				nil,
			)
			copyTask.End()
			if err != nil {
				return errors.Wrap(err, "copy manifest block DAG")
			}
			logObjectRefAccountingFields(accessCtx, "local-manifest", localRef)

			if !t.c.conf.GetDisableStoreManifest() {
				storeCtx, storeTask := trace.NewTask(accessCtx, "bldr/plugin-host-scheduler/download-manifest/store-local-ref")
				trace.Log(storeCtx, "accounting-phase", "world-op-store-local-manifest-ref")
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
			if _, err := ws.Sync(syncCtx); err != nil {
				syncTask.End()
				return errors.Wrap(err, "sync local manifest blocks")
			}
			syncTask.End()
			t.setManifestCopyStatus(manifestCopyPhaseDone, class, manifestSnapshot)
			le.Info("manifest download complete")
			return nil
		})
	})
	accessTask.End()
	return err
}
