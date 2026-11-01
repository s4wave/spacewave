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
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/volume"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

const (
	// FileName is the devtool database's file name inside a .bldr directory.
	FileName = "devtool.s4wave"
	// BucketID is the bucket that holds the World's blocks.
	BucketID = "bldr/devtool"
	// ObjectStoreID is the object store that holds the World head.
	ObjectStoreID = "bldr/devtool"
	// PluginHostObjectKey is the plugin host object the manifests link from.
	PluginHostObjectKey = "devtool"
	// headKey is the object store key of the World head state.
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

// OpenWorld opens the devtool World engine over vol. With create it writes an
// empty World with the plugin host's manifest store; otherwise it reads the
// stored head and rejects an empty World. Each commit stores the new head in
// the volume. The caller closes the engine.
func OpenWorld(ctx context.Context, le *logrus.Entry, vol volume.Volume, create bool) (*world_block.Engine, error) {
	// Register the gzip step and build the World transform config.
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_gzip.NewStepFactory())
	transformConf, err := block_transform.NewConfig([]config.Config{&transform_gzip.Config{}})
	if err != nil {
		return nil, errors.Wrap(err, "build transform config")
	}

	// Choose the head reference: a new one, or the stored one.
	headRef := &bucket.ObjectRef{BucketId: BucketID, TransformConf: transformConf}
	if !create {
		headRef, err = loadHeadRef(ctx, vol)
		if err != nil {
			return nil, errors.Wrap(err, "load head ref")
		}
		if headRef.GetRootRef().GetEmpty() {
			return nil, errors.New("devtool world is empty")
		}
		if headRef.GetBucketId() == "" {
			headRef.BucketId = BucketID
		}
	}

	// Build the block transformer and the bucket cursor over the head.
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, sfs, transformConf)
	if err != nil {
		return nil, errors.Wrap(err, "build block transformer")
	}
	cursor := bucket_lookup.NewCursor(ctx, nil, le, sfs, vol, xfrm, headRef, &bucket.BucketOpArgs{BucketId: BucketID}, transformConf)

	// Write the initial empty World for a new volume.
	if create {
		btx, bcs := cursor.BuildTransaction(nil)
		bcs.ClearAllRefs()
		bcs.SetBlock(world_block.NewWorld(false), true)
		rootRef, _, err := btx.Write(ctx, true)
		if err != nil {
			return nil, errors.Wrap(err, "write initial world")
		}
		headRef.RootRef = rootRef
		cursor.SetRootRef(rootRef)
		if err := writeHeadRef(ctx, vol, headRef); err != nil {
			return nil, errors.Wrap(err, "write head ref")
		}
	}

	// Build the World engine that stores each new head in the volume.
	commitFn := func(ctx context.Context, _ *bucket.ObjectRef, ref *bucket.ObjectRef) error {
		return writeHeadRef(ctx, vol, ref)
	}
	eng, err := world_block.NewEngine(ctx, le, cursor, world.NewSchema(bldr_manifest_world.WorldTypes).LookupOp, commitFn, false)
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

// loadHeadRef reads the World head ref from the volume's object store.
func loadHeadRef(ctx context.Context, vol volume.Volume) (*bucket.ObjectRef, error) {
	// Open a read transaction on the engine's object store.
	store, rel, err := vol.AccessObjectStore(ctx, ObjectStoreID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access object store")
	}
	defer rel()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "open object store tx")
	}
	defer tx.Discard()

	// Read the stored head state.
	data, found, err := tx.Get(ctx, []byte(headKey))
	if err != nil {
		return nil, errors.Wrap(err, "read world-head")
	}
	if !found {
		return nil, errors.Errorf("world-head not found in object store %s", ObjectStoreID)
	}

	// Decode the head reference.
	state := &world_block_engine.HeadState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal head state")
	}
	return state.GetHeadRef(), nil
}

// writeHeadRef stores ref as the World head in the volume's object store.
func writeHeadRef(ctx context.Context, vol volume.Volume, ref *bucket.ObjectRef) error {
	// Open a write transaction on the engine's object store.
	store, rel, err := vol.AccessObjectStore(ctx, ObjectStoreID, nil)
	if err != nil {
		return errors.Wrap(err, "access object store")
	}
	defer rel()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open object store tx")
	}
	defer tx.Discard()

	// Store the encoded head state.
	state := &world_block_engine.HeadState{HeadRef: ref}
	data, err := state.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal head state")
	}
	if err := tx.Set(ctx, []byte(headKey), data); err != nil {
		return errors.Wrap(err, "write world-head")
	}
	return tx.Commit(ctx)
}
