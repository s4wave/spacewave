//go:build !js

package devtool_store

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// ExportManifestSets writes a devtool database holding only refs, read from
// src, to dir/FileName.
//
// The database is written under a temporary name, closed, and renamed into
// place, so a reader of dir/FileName sees the previous export or this one,
// never a database another process is writing. A failed export removes its
// temporary file and leaves the previous export in place. The result opens
// with OpenVolume and LookupManifestSet like the .bldr directory it came from.
func ExportManifestSets(
	ctx context.Context,
	le *logrus.Entry,
	src world.WorldState,
	refs []*bldr_manifest.ManifestRef,
	dir string,
) error {
	// Write the database under a temporary name beside its final one.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errors.Wrap(err, "mkdir export dir")
	}
	path := filepath.Join(dir, FileName)
	tmpPath := path + ".tmp"
	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove stale export")
	}
	if err := writeManifestSets(ctx, le, src, refs, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	// Publish the closed database and make the rename durable.
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return errors.Wrap(err, "rename export into place")
	}
	return syncDir(dir)
}

// writeManifestSets creates the devtool database at path, copies the block
// graph of each ref into it and stores each ref under the plugin host, then
// closes it. The volume commits each write durably, so a closed database is
// complete on disk.
func writeManifestSets(
	ctx context.Context,
	le *logrus.Entry,
	src world.WorldState,
	refs []*bldr_manifest.ManifestRef,
	path string,
) (rerr error) {
	// Create the database and its World.
	vol, err := OpenVolumeFile(ctx, le, path, true)
	if err != nil {
		return err
	}
	defer func() {
		if err := vol.Close(); err != nil && rerr == nil {
			rerr = errors.Wrap(err, "close export volume")
		}
	}()
	eng, err := OpenWorld(ctx, le, vol, true)
	if err != nil {
		return err
	}
	defer eng.Close()
	dst := world.NewEngineWorldState(eng, true)
	dstBlocks := volume_controller.NewBucketStore(vol, &bucket.Config{Id: BucketID, Rev: 1})

	// Copy each manifest's stored blocks without decoding them, then store
	// the manifest under the plugin host as a build does.
	for _, ref := range refs {
		objRef := ref.GetManifestRef()
		err := src.AccessWorldState(ctx, objRef, func(bls *bucket_lookup.Cursor) error {
			return block.CopyGraph(ctx, bls.GetBucket(), dstBlocks, objRef.GetRootRef(), nil)
		})
		if err != nil {
			return errors.Wrapf(err, "copy manifest %s", ref.GetMeta().GetManifestId())
		}
		_, _, err = dst.ApplyWorldOp(
			ctx,
			bldr_manifest_world.NewStoreManifestOp(
				bldr_manifest.NewManifestArtifactKey(objRef),
				[]string{PluginHostObjectKey},
				ref,
			),
			vol.GetPeerID(),
		)
		if err != nil {
			return errors.Wrapf(err, "store manifest %s", ref.GetMeta().GetManifestId())
		}
	}
	return nil
}

// syncDir flushes a directory's entries, so a rename in it survives a crash.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
