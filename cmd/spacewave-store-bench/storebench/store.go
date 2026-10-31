//go:build !js

package storebench

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/storage"
	storage_controller "github.com/s4wave/spacewave/bldr/storage/controller"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	bucket_lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	bucket_setup "github.com/s4wave/spacewave/db/bucket/setup"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// storageID identifies the store's s4db storage on the bus.
const storageID = "default"

// Store is the World of a server store, opened with the composition a server
// uses: s4db storage, a local provider running as the peer volume's peer, and
// the World engine of a SharedObject in one of its accounts.
type Store struct {
	// Engine is the running World engine.
	Engine world.Engine
	// Volume is the account volume that holds the World.
	Volume volume.Volume
	// rels release the composition in reverse order.
	rels []func()
}

// OpenStore opens the World conf locates. When create is set and the account
// has no SharedObject with the configured ID, it creates an empty one. Close
// releases the store.
func OpenStore(ctx context.Context, le *logrus.Entry, conf *StoreConfig, create bool) (_ *Store, rerr error) {
	// Size the decoded block cache before the World fills it.
	if conf.BlockCacheBytes > 0 {
		block.SetSharedDecodedBlockCacheMaxCost(conf.BlockCacheBytes)
	}

	// Construct the bus and release what was built when a step fails.
	s := &Store{}
	defer func() {
		if rerr != nil {
			s.Close()
		}
	}()
	b, sr, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		return nil, errors.Wrap(err, "construct controller bus")
	}
	s.rels = append(s.rels, func() { _ = b.Close() })

	// Register the factories the storage and World controllers resolve.
	sr.AddFactory(node_controller.NewFactory(b))
	sr.AddFactory(bucket_lookup_concurrent.NewFactory(b))
	sr.AddFactory(bucket_setup.NewFactory(b))
	sr.AddFactory(storage_volume.NewFactory(b))
	sr.AddFactory(block_store_bucket.NewFactory(b))
	sr.AddFactory(provider_local.NewFactory(b))
	sr.AddFactory(sobject_world_engine.NewFactory(b))

	// Start the node and configset controllers.
	for _, c := range []config.Config{&node_controller.Config{}, &configset_controller.Config{}} {
		_, _, ref, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(c), nil)
		if err != nil {
			return nil, errors.Wrap(err, "start "+c.GetConfigID())
		}
		s.rels = append(s.rels, ref.Release)
	}

	// Attach the s4db storage with the configured volume caches.
	st := &sizedS4db{Storage: storage_native.NewS4db(false, conf.Dir), conf: conf}
	st.AddFactories(b, sr)
	storageCtrl := storage_controller.BuildStorageController(
		storageID,
		[]storage.Storage{st},
		controller.NewInfo("storebench/storage", controller.MustParseVersion("0.0.1"), "store bench storage"),
	)
	storageRel, err := b.AddController(ctx, storageCtrl, nil)
	if err != nil {
		return nil, errors.Wrap(err, "attach storage controller")
	}
	s.rels = append(s.rels, storageRel)

	// Start the peer volume and the local provider as its peer.
	volCtrl, volRef, err := storage_volume.ExecVolumeController(ctx, b, &storage_volume.Config{
		StorageId:       storageID,
		StorageVolumeId: conf.PeerVolumeID,
		VolumeConfig:    &volume_controller.Config{},
	})
	if err != nil {
		return nil, errors.Wrap(err, "start volume "+conf.PeerVolumeID)
	}
	s.rels = append(s.rels, volRef.Release)
	vol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "open volume "+conf.PeerVolumeID)
	}
	_, _, provRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local",
		PeerId:     vol.GetPeerID().String(),
		StorageId:  storageID,
	}), nil)
	if err != nil {
		return nil, errors.Wrap(err, "start local provider")
	}
	s.rels = append(s.rels, provRef.Release)

	// Open the account and find, or create, the World's SharedObject.
	acc, accRef, err := provider.ExAccessProviderAccount(ctx, b, "local", conf.AccountID, false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "open account "+conf.AccountID)
	}
	s.rels = append(s.rels, accRef.Release)
	if localAcc, ok := acc.(*provider_local.ProviderAccount); ok {
		s.Volume = localAcc.GetVolume()
	}
	ref, err := findSharedObject(ctx, acc, conf, create)
	if err != nil {
		return nil, err
	}

	// Serve the World from the SharedObject.
	eng, engRef, err := world.StartEngine(ctx, b, sobject_world_engine.NewConfig(conf.EngineID, ref), nil)
	if err != nil {
		return nil, errors.Wrap(err, "start world engine")
	}
	s.rels = append(s.rels, engRef.Release)
	s.Engine = eng
	return s, nil
}

// Close releases the World engine and the composition under it.
func (s *Store) Close() {
	for _, rel := range slices.Backward(s.rels) {
		rel()
	}
	s.rels = nil
}

// findSharedObject returns the SharedObject conf names in acc, creating it
// when create is set and the account does not hold it.
func findSharedObject(ctx context.Context, acc provider.ProviderAccount, conf *StoreConfig, create bool) (*sobject.SharedObjectRef, error) {
	// Read the account's SharedObject list.
	objects, err := sobject.GetSharedObjectProviderAccountFeature(ctx, acc)
	if err != nil {
		return nil, errors.Wrap(err, "open shared objects")
	}
	list, rel, err := objects.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access shared object list")
	}
	defer rel()
	snap, err := list.WaitValue(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "read shared object list")
	}

	// Return the configured SharedObject when the account holds it.
	entries := snap.GetSharedObjects()
	i := slices.IndexFunc(entries, func(e *sobject.SharedObjectListEntry) bool {
		return e.GetRef().GetProviderResourceRef().GetId() == conf.SharedObjectID
	})
	if i >= 0 {
		return entries[i].GetRef(), nil
	}
	if !create {
		return nil, errors.New("account " + conf.AccountID + " has no shared object " + conf.SharedObjectID)
	}

	// Create an empty SharedObject for a seeded store.
	ref, err := objects.CreateSharedObject(ctx, conf.SharedObjectID, &sobject.SharedObjectMeta{BodyType: conf.BodyType}, "", "")
	if err != nil {
		return nil, errors.Wrap(err, "create shared object "+conf.SharedObjectID)
	}
	return ref, nil
}

// sizedS4db is the s4db storage with the configured index page cache and
// checkpoint size on each volume. The store is the file's only writer.
type sizedS4db struct {
	storage.Storage
	// conf holds the volume sizes.
	conf *StoreConfig
}

// BuildVolumeConfig creates the volume config for the store ID with the
// configured sizes.
func (s *sizedS4db) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	conf, err := s.Storage.BuildVolumeConfig(id, baseVolCtrlConf)
	if volumeConf, ok := conf.(*volume_s4db.Config); ok {
		if s.conf.CacheBytes > 0 {
			volumeConf.CacheBytes = s.conf.CacheBytes
		}
	}
	return conf, err
}
