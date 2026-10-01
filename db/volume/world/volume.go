package volume_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_vlogger "github.com/s4wave/spacewave/db/kvtx/vlogger"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the World object volume controller.
const ControllerID = "hydra/volume/world"

// Version is the World volume implementation version.
var Version = controller.MustParseVersion("0.0.1")

// Volume stores its transactional metadata and blocks in one World object.
type Volume struct {
	*common_kvtx.Volume
	engine world.Engine
}

// NewVolume opens a World-backed volume. Every metadata transaction reads and
// publishes its object head within the same World transaction.
func NewVolume(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	conf *Config,
) (*Volume, error) {
	return NewVolumeWithEngine(ctx, le, b, sfs, conf, world.NewBusEngine(ctx, b, conf.GetEngineId()))
}

// NewVolumeWithEngine opens a Volume through an already-granted World capability.
// The caller retains the engine until the Volume closes.
func NewVolumeWithEngine(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	conf *Config,
	engine world.Engine,
) (*Volume, error) {
	// Build the key codec and the world-backed store.
	keys, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}
	store := &worldStore{engine: engine, b: b, le: le, sfs: sfs, conf: conf.CloneVT()}

	// Wrap the store in a logger when verbose logging is enabled.
	var loggedStore kvtx.Store = store
	if conf.GetVerbose() {
		loggedStore = kvtx_vlogger.NewVLogger(le, store)
	}

	// Construct the common kvtx volume over the logged store.
	bvol, err := common_kvtx.NewVolume(
		ctx, ControllerID, keys, loggedStore, conf.GetStoreConfig(),
		conf.GetNoGenerateKey(), conf.GetNoWriteKey(), nil, nil,
		func() error {
			// Delete the volume's object in one engine transaction.
			tx, err := engine.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			defer tx.Discard()
			if _, err := tx.DeleteObject(ctx, conf.GetObjectKey()); err != nil {
				return err
			}
			return tx.Commit(ctx)
		},
	)
	if err != nil {
		return nil, err
	}
	return &Volume{Volume: bvol, engine: engine}, nil
}

// Sync fences both Volume blocks and the enclosing World's durable head.
func (v *Volume) Sync(ctx context.Context) (bool, error) {
	return v.engine.Sync(ctx)
}

var _ volume.Volume = (*Volume)(nil)
