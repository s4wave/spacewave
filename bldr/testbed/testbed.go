package testbed

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/s4wave/spacewave/bldr/core"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	default_storage "github.com/s4wave/spacewave/bldr/storage/default"
	storage_inmem "github.com/s4wave/spacewave/bldr/storage/inmem"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/go-git/go-billy/v6"
	"github.com/sirupsen/logrus"
)

// Testbed contains a test environment for running plugins in-memory simulating a plugin host.
type Testbed struct {
	// ctx contains the context
	ctx context.Context
	// b contains the bus
	b bus.Bus
	// le contains the root logger
	le *logrus.Entry
	// sr contains the static resolver for the plugin host
	sr *static.Resolver
	// worldEngineID is the world engine id for the devtool world
	worldEngineID string
	// engineBucketID is the bucket used for world engine state storage
	engineBucketID string
	// engineObjectStoreID is the bucket used for root world engine state ref
	engineObjectStoreID string
	// vol is the volume used for state
	vol volume.Volume
	// volInfo is the volume info for the vol used for state
	volInfo *volume.VolumeInfo
	// volCtrl is the volume controller used for state
	volCtrl volume.Controller
	// peerID is the peerID to use for operations.
	peerID peer.ID
	// pluginHostID is the plugin host ID.
	pluginHostID string
	// pluginHostObjKey is the plugin host object key.
	pluginHostObjKey string
	// worldEngine is the world engine instance.
	worldEngine world.Engine
	// worldState is the world state instance.
	worldState world.WorldState
	// scheduler is the plugin scheduler controller.
	scheduler *plugin_host_scheduler.Controller
	// mux is the RPC service mux
	mux srpc.Mux
	// rpcServiceCtrl is the RPC service controller
	rpcServiceCtrl *bifrost_rpc.RpcServiceController
	// rels are the release funcs
	rels []func()
}

// BuildTestbed builds the testbed constructing an in-memory volume and plugin host.
// Returns a set of functions to call to release the controllers.
// If stateRoot is empty, uses a temporary directory.
func BuildTestbed(rctx context.Context, le *logrus.Entry) (*Testbed, error) {
	ctx, ctxCancel := context.WithCancel(rctx)
	var rels []func()
	rel := func() {
		for _, fn := range rels {
			fn()
		}
		ctxCancel()
	}

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		rel()
		return nil, err
	}

	// add controller factories
	sr.AddFactory(storage_volume.NewFactory(b))
	sr.AddFactory(world_block_engine.NewFactory(b))
	sr.AddFactory(plugin_host_scheduler.NewFactory(b))

	// add the configset controller
	configSetCtrl, _ := configset_controller.NewController(le, b)
	relConfigSetCtrl, err := b.AddController(ctx, configSetCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relConfigSetCtrl)

	// attach the inmem storage controller
	storageID := default_storage.StorageID
	storageCtrl := storage_inmem.NewController(storageID)
	relStorageCtrl, err := b.AddController(ctx, storageCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relStorageCtrl)

	volCtrl, volCtrlRef, err := storage_volume.ExecVolumeController(ctx, b, &storage_volume.Config{
		StorageId:       storageID,
		StorageVolumeId: "devtool",
		VolumeConfig: &volume_controller.Config{
			VolumeIdAlias: []string{"dist"},
		},
	})
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, volCtrlRef.Release)

	vol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		rel()
		return nil, err
	}

	volInfo, err := volume.NewVolumeInfo(ctx, volCtrl.GetControllerInfo(), vol)
	if err != nil {
		rel()
		return nil, err
	}

	// start the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, nodeCtrlRef, err := bus.ExecOneOff(ctx, b, dir, nil, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, nodeCtrlRef.Release)

	// start devtool world
	engineBucketID := "bldr/devtool"
	engineObjStoreID := engineBucketID
	engineID := "bldr"

	// create bucket if it doesn't exist
	bucketConf, err := bucket.NewConfig(engineBucketID, 1, nil, nil)
	if err != nil {
		rel()
		return nil, err
	}
	_, err = bucket.ExApplyBucketConfig(ctx, b, bucket.NewApplyBucketConfigToVolume(bucketConf, vol.GetID()))
	if err != nil {
		rel()
		return nil, err
	}

	initRef := &bucket.ObjectRef{BucketId: engineBucketID}
	engConf := world_block_engine.NewConfig(
		engineID,
		vol.GetID(), engineBucketID,
		engineObjStoreID,
		initRef,
		nil,
		false,
	)

	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		b,
		engConf,
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, worldCtrlRef.Release)

	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		rel()
		return nil, err
	}
	worldState := world.NewEngineWorldState(eng, true)

	// register the world operation types for plugin host
	lookupOpCtrl := world.NewLookupOpController("bldr-plugin-host-ops", engineID, bldr_manifest_world.LookupOp)
	relLookupCtrl, err := b.AddController(ctx, lookupOpCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relLookupCtrl)

	// create the manifest store in the engine
	pluginHostID := "plugin-host"
	pluginHostObjKey := pluginHostID
	if _, err := bldr_manifest_world.CreateManifestStoreInEngine(ctx, eng, pluginHostObjKey); err != nil {
		rel()
		return nil, err
	}

	// load the plugin scheduler
	sched, _, schedRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_scheduler.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(plugin_host_scheduler.NewConfig(
			engineID,
			pluginHostObjKey,
			vol.GetID(),
			vol.GetPeerID().String(),
			true,
			false,
			false,
		)),
		nil,
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, schedRef.Release)

	// create the RPC mux
	mux := srpc.NewMux()

	// register the rpc service controller
	rpcServiceCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo("testbed/rpc-host", semver.MustParse("0.0.1"), "rpc host for testbed"),
		func(ctx context.Context, released func()) (srpc.Invoker, func(), error) {
			return mux, nil, nil
		},
		nil,
		false,
		nil,
		nil,
		nil,
	)
	rpcServiceCtrlRel, err := b.AddController(ctx, rpcServiceCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, rpcServiceCtrlRel)

	return &Testbed{
		ctx:                 ctx,
		b:                   b,
		le:                  le,
		sr:                  sr,
		worldEngineID:       engineID,
		engineBucketID:      engineBucketID,
		engineObjectStoreID: engineObjStoreID,
		vol:                 vol,
		volInfo:             volInfo,
		volCtrl:             volCtrl,
		peerID:              vol.GetPeerID(),
		pluginHostID:        pluginHostID,
		pluginHostObjKey:    pluginHostObjKey,
		worldEngine:         eng,
		worldState:          worldState,
		scheduler:           sched,
		mux:                 mux,
		rpcServiceCtrl:      rpcServiceCtrl,
		rels:                rels,
	}, nil
}

// GetContext returns the context.
func (d *Testbed) GetContext() context.Context {
	return d.ctx
}

// GetBus returns the bus.
func (d *Testbed) GetBus() bus.Bus {
	return d.b
}

// GetLogger returns the root logger
func (d *Testbed) GetLogger() *logrus.Entry {
	return d.le
}

// GetStaticResolver returns the static controller resolver.
func (d *Testbed) GetStaticResolver() *static.Resolver {
	return d.sr
}

// GetVolume returns the storage volume in use.
func (d *Testbed) GetVolume() volume.Volume {
	return d.vol
}

// GetVolumeInfo returns the storage volume info.
func (d *Testbed) GetVolumeInfo() *volume.VolumeInfo {
	return d.volInfo
}

// GetVolumeController returns the storage volume controller in use.
func (d *Testbed) GetVolumeController() volume.Controller {
	return d.volCtrl
}

// GetWorldEngineID returns the world engine id.
func (d *Testbed) GetWorldEngineID() string {
	return d.worldEngineID
}

// GetWorldEngine returns the world engine instance.
func (d *Testbed) GetWorldEngine() world.Engine {
	return d.worldEngine
}

// GetWorldState returns the world state handle.
func (d *Testbed) GetWorldState() world.WorldState {
	return d.worldState
}

// GetPluginHostId returns the plugin host ID.
func (d *Testbed) GetPluginHostId() string {
	return d.pluginHostID
}

// GetPluginHostObjKey returns the plugin host object key.
func (d *Testbed) GetPluginHostObjKey() string {
	return d.pluginHostObjKey
}

// GetScheduler returns the plugin scheduler controller.
func (d *Testbed) GetScheduler() *plugin_host_scheduler.Controller {
	return d.scheduler
}

// GetMux returns the RPC service mux.
func (d *Testbed) GetMux() srpc.Mux {
	return d.mux
}

// GetRpcServiceCtrl returns the RPC service controller.
func (d *Testbed) GetRpcServiceCtrl() *bifrost_rpc.RpcServiceController {
	return d.rpcServiceCtrl
}

// CreateManifestWithBilly creates a manifest with billyfs and links it to the plugin host.
// This is used by end to end tests to create plugin manifests ad-hoc.
// distFs and assetsFs can both be nil to create empty fs.
func (d *Testbed) CreateManifestWithBilly(
	ctx context.Context,
	manifestMeta *bldr_manifest.ManifestMeta,
	entrypoint string,
	distFs, assetsFs billy.Filesystem,
	ts *timestamppb.Timestamp,
) (manifest *bldr_manifest.Manifest, manifestRef *bldr_manifest.ManifestRef, err error) {
	err = d.GetWorldEngine().AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		btx, bcs := bls.BuildTransactionAtRef(nil, nil)

		manifest, err = bldr_manifest.CreateManifestWithBilly(ctx, bcs, manifestMeta, entrypoint, distFs, assetsFs, ts)
		if err != nil {
			return err
		}

		manifestBlockRef, _, err := btx.Write(ctx, true)
		if err != nil {
			return err
		}

		manifestObjRef := bls.GetRef().Clone()
		manifestObjRef.RootRef = manifestBlockRef
		manifestRef = bldr_manifest.NewManifestRef(manifestMeta, manifestObjRef)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	// link it with the plugin host
	err = bldr_manifest_world.ExStoreManifestOp(
		ctx,
		d.GetWorldState(),
		d.GetVolume().GetPeerID(),
		"manifests/"+manifestMeta.GetManifestId(),
		[]string{d.GetPluginHostObjKey()},
		manifestRef,
	)
	if err != nil {
		return nil, nil, err
	}

	return manifest, manifestRef, nil
}

// Release releases the devtool bus.
func (d *Testbed) Release() {
	for _, rel := range d.rels {
		rel()
	}
}
