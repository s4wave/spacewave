package plugin_host_scheduler

import (
	"context"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

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
	trace.Log(ctx, "plugin-id", t.pluginID)
	trace.Log(ctx, "manifest-ref", ref.MarshalString())
	trace.Log(ctx, "startup-fetch-kind", "background-manifest-dag-copy")

	ws, err := t.c.worldStateCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// Access the world root bucket (dest) then the manifest source bucket (src).
	return ws.AccessWorldState(ctx, nil, func(dest *bucket_lookup.Cursor) error {
		return ws.AccessWorldState(ctx, ref, func(src *bucket_lookup.Cursor) error {
			destBucketID := dest.GetOpArgs().GetBucketId()
			le.Infof("copying manifest DAG from bucket %s to %s", src.GetOpArgs().GetBucketId(), destBucketID)
			localRef, err := bucket_lookup.CopyObjectToBucket(
				ctx,
				dest,
				src,
				bldr_manifest.NewManifestBlock,
				1,
				false,
				nil,
			)
			if err != nil {
				return errors.Wrap(err, "copy manifest block DAG")
			}
			if !t.c.conf.GetDisableStoreManifest() {
				manifestKey := bldr_manifest.NewManifestKey(t.c.objKey, manifestMeta)
				if err := bldr_manifest_world.ExStoreManifestOp(
					ctx,
					ws,
					t.c.peerID,
					manifestKey,
					[]string{t.c.objKey},
					bldr_manifest.NewManifestRef(manifestMeta, localRef),
				); err != nil {
					return errors.Wrap(err, "store local manifest ref")
				}
			}
			if _, err := ws.Sync(ctx); err != nil {
				return errors.Wrap(err, "sync local manifest blocks")
			}
			le.Info("manifest download complete")
			return nil
		})
	})
}
