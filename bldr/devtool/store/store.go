//go:build !js

// Package devtool_store opens the devtool database a bldr build writes to
// .bldr/devtool.s4wave: an s4db volume whose World holds the manifests built
// for the plugin host object.
package devtool_store

import (
	"context"
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
