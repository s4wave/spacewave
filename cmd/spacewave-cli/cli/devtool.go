//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/volume"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

const (
	// devtoolEngineBucketID is the bucket ID used by the devtool world engine.
	devtoolEngineBucketID = "bldr/devtool"
	// devtoolEngineObjStoreID is the object store ID used by the devtool world engine.
	devtoolEngineObjStoreID = "bldr/devtool"
	// devtoolPluginHostObjectKey is the object key for the devtool plugin host.
	devtoolPluginHostObjectKey = "devtool"
)

// openDevtoolVolume opens the devtool bolt volume at the given .bldr/ path.
func openDevtoolVolume(ctx context.Context, le *logrus.Entry, bldrPath string) (volume.Volume, error) {
	dbPath := filepath.Join(bldrPath, "devtool.s4wave")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, errors.Errorf("devtool database not found at %s", dbPath)
	}
	conf := &volume_bolt.Config{
		Path:          dbPath,
		NoGenerateKey: true,
		NoWriteKey:    true,
	}
	return volume_bolt.NewBolt(ctx, le, conf)
}

// buildStepFactorySet builds the block transform step factory set with s2 support.
func buildStepFactorySet() *block_transform.StepFactorySet {
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_s2.NewStepFactory())
	return sfs
}

// loadDevtoolHeadRef reads the world engine head ref from the volume's object store.
func loadDevtoolHeadRef(ctx context.Context, vol volume.Volume) (*bucket.ObjectRef, error) {
	store, rel, err := vol.AccessObjectStore(ctx, devtoolEngineObjStoreID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access object store")
	}
	defer rel()

	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "open object store tx")
	}
	defer tx.Discard()

	data, found, err := tx.Get(ctx, []byte("world-head"))
	if err != nil {
		return nil, errors.Wrap(err, "read world-head")
	}
	if !found {
		return nil, errors.Errorf("world-head not found in object store %s", devtoolEngineObjStoreID)
	}

	state := &world_block_engine.HeadState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal head state")
	}
	return state.GetHeadRef(), nil
}

// lookupDevtoolManifest opens the devtool world and finds a manifest by ID.
func lookupDevtoolManifest(
	ctx context.Context,
	le *logrus.Entry,
	vol volume.Volume,
	manifestID string,
) (*bldr_manifest_world.CollectedManifest, error) {
	headRef, err := loadDevtoolHeadRef(ctx, vol)
	if err != nil {
		return nil, errors.Wrap(err, "load head ref")
	}
	if headRef.GetRootRef().GetEmpty() {
		return nil, errors.New("devtool world is empty (no head ref)")
	}

	if headRef.GetBucketId() == "" {
		headRef.BucketId = devtoolEngineBucketID
	}

	sfs := buildStepFactorySet()

	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_s2.Config{},
	})
	if err != nil {
		return nil, errors.Wrap(err, "build transform config")
	}

	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		sfs,
		transformConf,
	)
	if err != nil {
		return nil, errors.Wrap(err, "build block transformer")
	}

	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		le,
		sfs,
		vol,
		xfrm,
		headRef,
		&bucket.BucketOpArgs{
			BucketId: devtoolEngineBucketID,
		},
		transformConf,
	)

	eng, err := world_block.NewEngine(
		ctx,
		le,
		cursor,
		bldr_manifest_world.LookupOp,
		nil,
		false,
	)
	if err != nil {
		return nil, errors.Wrap(err, "build world engine")
	}

	ws := world.NewEngineWorldState(eng, false)

	manifests, _, err := bldr_manifest_world.CollectManifests(ctx, ws, nil, devtoolPluginHostObjectKey)
	if err != nil {
		return nil, errors.Wrap(err, "collect manifests")
	}

	list, ok := manifests[manifestID]
	if !ok || len(list) == 0 {
		available := make([]string, 0, len(manifests))
		for id := range manifests {
			available = append(available, id)
		}
		return nil, errors.Errorf("manifest %q not found (available: %v)", manifestID, available)
	}

	return list[0], nil
}
