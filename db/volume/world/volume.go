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
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
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

	// Initialize identity while holding the writer that checks its absence.
	// A second attachment must use the first committed identity.
	if !conf.GetNoGenerateKey() && !conf.GetNoWriteKey() {
		if err := initializeIdentity(ctx, store, keys.GetPeerPrivKey()); err != nil {
			return nil, err
		}
	}

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

// initializeIdentity creates the durable identity once across concurrent mounts.
func initializeIdentity(ctx context.Context, store kvtx.Store, key []byte) error {
	// Open a write transaction and check the identity key's absence.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, key)
	if err != nil || (found && len(data) != 0) {
		return err
	}

	// Generate a new peer identity and marshal its private key.
	p, err := peer.NewPeer(nil)
	if err != nil {
		return err
	}
	priv, err := p.GetPrivKey(ctx)
	if err != nil {
		return err
	}
	data, err = keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		return err
	}

	// Store the identity and commit the transaction.
	if err := tx.Set(ctx, key, data); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Sync fences both Volume blocks and the enclosing World's durable head.
func (v *Volume) Sync(ctx context.Context) (bool, error) {
	return v.engine.Sync(ctx)
}

var _ volume.Volume = (*Volume)(nil)
