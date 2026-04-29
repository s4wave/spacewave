//go:build !js

package cli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/volume"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

// openDevtoolVolume opens the devtool bolt volume at sourcePath.
func openDevtoolVolume(ctx context.Context, le *logrus.Entry, sourcePath string) (volume.Volume, error) {
	dbPath := filepath.Join(sourcePath, "devtool.s4wave")
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

// loadHeadRef reads the world engine head ref from the volume's object store.
func loadHeadRef(ctx context.Context, vol volume.Volume) (*bucket.ObjectRef, error) {
	store, rel, err := vol.AccessObjectStore(ctx, engineObjStoreID, nil)
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
		return nil, errors.Errorf("world-head not found in object store %s", engineObjStoreID)
	}

	state := &world_block_engine.HeadState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal head state")
	}
	return state.GetHeadRef(), nil
}
