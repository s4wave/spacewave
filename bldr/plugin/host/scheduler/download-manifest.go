package plugin_host_scheduler

import (
	"context"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	block_copy "github.com/s4wave/spacewave/db/block/copy"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/pkg/errors"
)

// execDownloadManifest copies manifest blocks from the source bucket to the world bucket.
func (t *pluginInstance) execDownloadManifest(ctx context.Context, manifestSnapshot *bldr_manifest.ManifestSnapshot) error {
	if t.c.conf.GetDisableCopyManifest() {
		return nil
	}

	le := t.le
	ref := manifestSnapshot.GetManifestRef()
	blockRef := ref.GetRootRef()
	if blockRef.GetEmpty() {
		return errors.New("manifest ref has empty root block ref")
	}

	ws, err := t.c.worldStateCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// Access the world root bucket (dest) then the manifest source bucket (src).
	return ws.AccessWorldState(ctx, nil, func(dest *bucket_lookup.Cursor) error {
		return ws.AccessWorldState(ctx, ref, func(src *bucket_lookup.Cursor) error {
			le.Infof("copying manifest DAG from bucket %s to %s", src.GetOpArgs().GetBucketId(), dest.GetOpArgs().GetBucketId())
			err := block_copy.CopyBlockDAG(ctx, blockRef, bldr_manifest.NewManifestBlock, src.GetBucket(), dest.GetBucket())
			if err != nil {
				return errors.Wrap(err, "copy manifest block DAG")
			}
			le.Info("manifest download complete")
			return nil
		})
	})
}
