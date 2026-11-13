//go:build !js

// Package devtool_store opens the devtool database a bldr build writes to
// .bldr/devtool.s4wave: an s4db volume whose World holds the manifests built
// for the plugin host object.
package devtool_store

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

const (
	// FileName is the devtool database's file name inside a .bldr directory.
	FileName = "devtool.s4wave"
	// PluginHostObjectKey is the plugin host object the manifests link from.
	PluginHostObjectKey = "devtool"
	// headKey is the object store key an older build kept the head under.
	headKey = "world-head"
)

// OpenVolume opens the devtool database of the .bldr directory at bldrPath
// without generating or writing its key.
func OpenVolume(ctx context.Context, le *logrus.Entry, bldrPath string) (volume.Volume, error) {
	return OpenVolumeFile(ctx, le, filepath.Join(bldrPath, FileName), false)
}

// OpenVolumeCopy opens a private copy of the devtool database of the .bldr or
// export directory at bldrPath, for a reader that must leave the source
// byte-identical, such as a deploy from an export whose checksums are
// recorded. Any s4db open writes the file, even a read-only one, which takes a
// reader slot and writes its pins. The copy is a copy-on-write clone where the
// OS temp dir shares a cloning filesystem with the source, such as APFS on
// macOS, and a byte copy otherwise. It stays in the OS temp dir, which the OS
// cleans when a killed command leaves it behind, so the source directory never
// gains a file. Close removes the copy.
func OpenVolumeCopy(ctx context.Context, le *logrus.Entry, bldrPath string) (volume.Volume, error) {
	// Copy the database into a new temporary directory.
	dir, err := os.MkdirTemp("", "devtool-copy-")
	if err != nil {
		return nil, errors.Wrap(err, "create copy dir")
	}
	path := filepath.Join(dir, FileName)
	if err := copyFile(filepath.Join(bldrPath, FileName), path); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}

	// Open the copy and remove it with the volume.
	vol, err := OpenVolumeFile(ctx, le, path, false)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &copyVolume{Volume: vol, dir: dir}, nil
}

// copyVolume is a volume over a private copy of a database file.
type copyVolume struct {
	volume.Volume
	dir string
}

// Close closes the volume and removes its copy.
func (v *copyVolume) Close() error {
	err := v.Volume.Close()
	if rerr := os.RemoveAll(v.dir); err == nil {
		err = rerr
	}
	return err
}

// copyFile copies the file at src to a new file at dst, as a clone when the
// filesystem can make one.
func copyFile(src, dst string) error {
	// Clone the source, which shares its blocks in constant time.
	if cloneFile(src, dst) == nil {
		return nil
	}

	// Otherwise open the source and create the copy.
	in, err := os.Open(src)
	if err != nil {
		return errors.Errorf("devtool database not found at %s", src)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.Wrap(err, "create devtool database copy")
	}

	// Copy the bytes and close the copy.
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return errors.Wrap(err, "copy devtool database")
}

// OpenVolumeFile opens the devtool database file at path. An existing file is
// opened without writing its key; create makes a new one with a new key.
func OpenVolumeFile(ctx context.Context, le *logrus.Entry, path string, create bool) (volume.Volume, error) {
	// Require the file unless creating it.
	if !create {
		if _, err := os.Stat(path); err != nil {
			return nil, errors.Errorf("devtool database not found at %s", path)
		}
	}

	// Open the s4db volume.
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{
		Path:          path,
		NoGenerateKey: !create,
		NoWriteKey:    !create,
	})
	if err != nil {
		return nil, errors.Wrap(err, "open devtool volume")
	}
	return vol, nil
}

// OpenWorld opens the devtool World engine over vol, through the bucket handle
// the devtool's engine controller uses, so both hold one head. With create it writes an
// empty World with the plugin host's manifest store; otherwise it reads the
// stored head and rejects an empty World. The World's head is a root of vol, and each commit swaps it with the commit's
// blocks in one transaction. The caller closes the engine.
func OpenWorld(ctx context.Context, le *logrus.Entry, vol volume.Volume, create bool) (*world_block.Engine, error) {
	// Register the gzip step and build the World transform config.
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_gzip.NewStepFactory())
	transformConf, err := block_transform.NewConfig([]config.Config{&transform_gzip.Config{}})
	if err != nil {
		return nil, errors.Wrap(err, "build transform config")
	}

	// Build the block transformer and the bucket cursor, and the authority
	// over the World's root.
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, sfs, transformConf)
	if err != nil {
		return nil, errors.Wrap(err, "build block transformer")
	}
	headRef := &bucket.ObjectRef{BucketId: BucketID, TransformConf: transformConf}
	store := volume_controller.NewBucketStore(vol, &bucket.Config{Id: BucketID, Rev: 1})
	cursor := bucket_lookup.NewCursor(ctx, nil, le, sfs, store, xfrm, headRef, &bucket.BucketOpArgs{BucketId: BucketID}, transformConf)
	authority := world_block.NewLocalAuthority(le, cursor, world_block.WorldRootName(EngineID), nil, coord.Scope{})

	// Read the head, adopting the head record an older build wrote.
	head, err := openHead(ctx, le, vol, authority, cursor, create)
	if err != nil {
		cursor.Release()
		return nil, err
	}

	// Build the World engine at the head.
	cursor.SetRootRef(head.Root)
	eng, err := world_block.NewEngine(ctx, le, cursor, world.NewSchema(bldr_manifest_world.WorldTypes).LookupOp, authority, false)
	if err != nil {
		return nil, errors.Wrap(err, "build world engine")
	}

	// Create the plugin host's manifest store in a new World.
	if create {
		ws := world.NewEngineWorldState(eng, true)
		if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, PluginHostObjectKey); err != nil {
			_ = eng.Close()
			return nil, errors.Wrap(err, "create manifest store")
		}
	}
	return eng, nil
}

// openHead returns the World's head. With create it writes an empty World
// when the root is free; otherwise a free root adopts the head record an older
// build kept in the object store, releasing the root that build held it under
// and deleting the record, as the devtool's engine controller does, or fails.
func openHead(
	ctx context.Context,
	le *logrus.Entry,
	vol volume.Volume,
	authority *world_block.LocalAuthority,
	cursor *bucket_lookup.Cursor,
	create bool,
) (world.Head, error) {
	// Open the object store an older build kept the head record in.
	store, rel, err := vol.AccessObjectStore(ctx, ObjectStoreID, nil)
	if err != nil {
		return world.Head{}, errors.Wrap(err, "access object store")
	}
	defer rel()

	// Adopt the older build's head, or write a new World.
	legacy := world_block_engine.LegacyHead{
		Store:    store,
		Key:      []byte(headKey),
		RootName: world_block_engine.LegacyRootName(ObjectStoreID, headKey),
	}
	return world_block_engine.OpenHead(ctx, le, authority, cursor, legacy, world_block_engine.OpenHeadOptions{
		Initial: func(ctx context.Context) (*block.BlockRef, error) {
			// Refuse an empty World unless creating one.
			if !create {
				return nil, errors.New("devtool world is empty")
			}

			// Write a new empty World.
			btx, bcs := cursor.BuildTransaction(nil)
			bcs.ClearAllRefs()
			initial := world_block.NewWorld(world_block.NewestWorldFormat, false)
			bcs.SetBlock(initial, true)
			root, _, err := btx.Write(ctx, true)
			return root, errors.Wrap(err, "write initial world")
		},
	})
}
