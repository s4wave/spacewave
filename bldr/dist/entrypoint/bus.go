package dist_entrypoint

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	"github.com/s4wave/spacewave/bldr/entrypoint/compose"
	entrypoint_state "github.com/s4wave/spacewave/bldr/entrypoint/state"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_entrypoint_controller "github.com/s4wave/spacewave/bldr/plugin/entrypoint/controller"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	default_storage "github.com/s4wave/spacewave/bldr/storage/default"
	"github.com/s4wave/spacewave/db/bucket"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// DistBus contains the distribution host bus.
type DistBus struct {
	// ctx contains the context
	ctx context.Context
	// b contains the bus
	b bus.Bus
	// le contains the root logger
	le *logrus.Entry
	// sr contains the static resolver
	sr *static.Resolver
	// platformID is the distribution platform id.
	platformID string
	// storageID is the id of the storage attached to the bus
	storageID string
	// worldEngineID is the world engine id for state
	worldEngineID string
	// engineBucketID is the bucket used for world engine state storage
	engineBucketID string
	// engineObjectStoreID is the bucket used for root world engine state ref
	engineObjectStoreID string
	// pluginHostObjectKey is the object key used for the PluginHost
	pluginHostObjectKey string
	// pluginSchedCtrl is the plugin scheduler
	pluginSchedCtrl *plugin_host_scheduler.Controller
	// pluginHostCtrl is the plugin host controller
	pluginHostCtrl *plugin_host_default.PluginHostController
	// stateRoot is the .bldr state root dir.
	stateRoot string
	// vol is the volume used for state
	vol volume.Volume
	// peerID is the peerID to use for operations.
	peerID peer.ID
	// worldEngine is the world engine instance.
	worldEngine world.Engine
	// worldState is the world state instance.
	worldState world.WorldState
	// rel is the release func
	rel func()
	// addRelease appends cleanup after bus-owned resources.
	addRelease func(func())
}

// distReleaseStack runs bus cleanup before caller callbacks in registration order.
type distReleaseStack struct {
	callbacks []func()
}

// add registers a cleanup operation for this distribution lifetime.
func (s *distReleaseStack) add(release func()) {
	if release != nil {
		s.callbacks = append(s.callbacks, release)
	}
}

// release runs every operation in the order it was registered.
func (s *distReleaseStack) release() {
	for _, release := range s.callbacks {
		release()
	}
}

// BuildDistBus builds the storage and bus for the distribution entrypoint.
// Returns a set of functions to call to release the controllers.
func BuildDistBus(
	rctx context.Context,
	le *logrus.Entry,
	distMeta *bldr_dist.DistMeta,
	stateRoot,
	webRuntimeID string,
	configSetProto *configset_proto.ConfigSet,
	staticBlockStoreReaderBuilder refcount.RefCountResolver[*kvfile.Reader],
	composition *compose.Composition,
	preBuildHooks []DistBusHook,
) (*DistBus, error) {
	// Log the distribution identity and bound the lifetime with a context.
	projectID := distMeta.GetProjectId()
	platformID := distMeta.GetPlatformId()
	le.
		WithFields(logrus.Fields{"project-id": projectID, "platform-id": platformID}).
		Info("initializing application and storage...")
	ctx, ctxCancel := context.WithCancel(rctx)

	// Build the release stack holding the distribution's cleanup order.
	rels := &distReleaseStack{}
	rels.add(ctxCancel)
	rel := rels.release
	addRelease := rels.add

	// The composition's factories must resolve the startup config set.
	b, sr, err := NewCoreBus(ctx, le)
	if err != nil {
		rel()
		return nil, err
	}
	composition.AddFactories(b, sr)

	// Construct the DistBus with the bus and storage identity.
	storageID := default_storage.StorageID
	distBus := &DistBus{
		ctx:        ctx,
		b:          b,
		le:         le,
		sr:         sr,
		storageID:  storageID,
		platformID: platformID,
		stateRoot:  stateRoot,
		rel:        rel,
		addRelease: addRelease,
	}

	// add the configset controller
	configSetCtrl, _ := configset_controller.NewController(le, b)
	_, err = b.AddController(ctx, configSetCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}

	// build the plugin state paths on disk
	pluginsRoot := filepath.Join(stateRoot, "p")
	pluginsDistRoot := filepath.Join(pluginsRoot, "d")
	pluginsStateRoot := filepath.Join(pluginsRoot, "s")

	// Web distribution platforms do not create local plugin-state paths.
	if !isWebDistPlatform(platformID) {
		if err := os.MkdirAll(pluginsDistRoot, 0o755); err != nil {
			rel()
			return nil, err
		}
		if err := os.MkdirAll(pluginsStateRoot, 0o755); err != nil {
			rel()
			return nil, err
		}
	}

	// Run the config set. The scheduler starts after its controllers attach.
	var configSet configset.ConfigSet
	if len(configSetProto.GetConfigs()) != 0 {
		configSet, err = configSetProto.Resolve(ctx, b)
		if err != nil {
			rel()
			return nil, err
		}
	}
	configSetAttached, releaseConfigSet, err := applyConfigSet(b, configSet)
	if err != nil {
		rel()
		return nil, err
	}
	rels.add(releaseConfigSet)

	// Run the pre-build hooks and retain their cleanup functions.
	for _, hook := range preBuildHooks {
		hookRels, err := hook(distBus)
		if err != nil {
			rel()
			return nil, err
		}
		for _, hookRel := range hookRels {
			rels.add(hookRel)
		}
	}

	// attach the default storage controller
	// this provides separate named volumes with the storage volume controller.
	storageCtrl := default_storage.NewController(storageID, b, stateRoot)
	relStorageCtrl, err := b.AddController(ctx, storageCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels.add(relStorageCtrl)

	// ensure there is at least one storage method
	storageMethods := storageCtrl.GetStorage()
	if len(storageMethods) == 0 {
		rel()
		return nil, errors.New("no available storage methods")
	}

	// add storage factories
	for _, st := range storageMethods {
		st.AddFactories(b, sr)
	}

	// Read the embedded world root and object key from the dist metadata.
	distBundleWorldRootRef := distMeta.GetDistWorldRef()
	distBundleObjKey := distMeta.GetDistObjectKey()

	// mount the embedded read-only block storage
	embedBlockStoreID := bldr_dist.StaticBlockStoreID
	staticBlockStoreCtrl := NewStaticBlockStore(
		le,
		b,
		embedBlockStoreID,
		staticBlockStoreReaderBuilder,
		store_kvkey.NewDefaultKVKey(),
		nil, // []string{distBundleBucketConf.GetId()},
		nil,
	)
	relStaticVolCtrl, err := b.AddController(ctx, staticBlockStoreCtrl, nil)
	if err != nil {
		rel()
		return nil, errors.Wrap(err, "add static block store controller")
	}
	rels.add(relStaticVolCtrl)

	// Native renderers share the daemon store; browser profiles keep their
	// distribution volume within the origin's storage namespace.
	volumeConf := entrypoint_state.NewVolumeConfig(storageID)
	if isWebDistPlatform(platformID) {
		volumeConf.StorageVolumeId = "dist/" + projectID
		volumeConf.VolumeConfig.VolumeIdAlias = []string{"dist"}
		volumeConf.VolumeConfig.DisablePeer = true
	}

	// Retain the state volume until the distribution bus has stopped.
	volCtrli, _, diRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(volumeConf),
		ctxCancel,
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels.add(diRef.Release)

	// Resolve the volume from the running controller.
	volCtrl, ok := volCtrli.(volume.Controller)
	if !ok {
		rel()
		return nil, errors.New("volume controller returned invalid value")
	}

	// Get the live volume handle for the distribution state.
	vol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		rel()
		return nil, err
	}

	// NewDistBucketConfig and the distribution compiler share the embedded
	// manifest bucket schema.
	distBundleBucketConf, err := bldr_dist.NewDistBucketConfig(projectID)
	if err != nil {
		rel()
		return nil, err
	}

	// Apply the embedded manifest bucket schema to the volume.
	_, _, _, err = vol.ApplyBucketConfig(ctx, distBundleBucketConf)
	if err != nil {
		rel()
		return nil, err
	}

	// mount the manifest kvtx block world backed by read-only storage
	distWorldEngineID := bldr_dist.DistWorldEngineID
	embedEngineConf := world_block_engine.NewConfig(
		distWorldEngineID,
		vol.GetID(),
		distBundleBucketConf.GetId(),
		"",
		distBundleWorldRootRef,
		nil,
		false,
	)
	embedEngineConf.DisableLookup = true

	// Start the embedded world engine controller.
	_, _, embedEngineCtrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(embedEngineConf),
		nil,
	)
	if err != nil {
		rel()
		return nil, errors.Wrap(err, "start static embedded engine controller")
	}
	rels.add(embedEngineCtrlRef.Release)

	// mount the manifest fetcher from the static world
	staticManifestFetcher := manifest_fetch_world.NewController(le, b, &manifest_fetch_world.Config{
		EngineId:   distWorldEngineID,
		ObjectKeys: []string{distBundleObjKey},
	})
	relStaticManifestFetcher, err := b.AddController(ctx, staticManifestFetcher, nil)
	if err != nil {
		rel()
		return nil, errors.Wrap(err, "start static manifest fetcher")
	}
	rels.add(relStaticManifestFetcher)

	// start the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, nodeCtrlRef, err := bus.ExecOneOff(ctx, b, dir, nil, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels.add(nodeCtrlRef.Release)

	// Open the shared project World independently of this renderer's plugin set.
	engConf, err := entrypoint_state.NewWorldConfig(projectID, vol.GetID())
	if err != nil {
		rel()
		return nil, err
	}
	engineID := engConf.GetEngineId()
	engineBucketID := engConf.GetBucketId()
	engineObjStoreID := engConf.GetObjectStoreId()
	bucketConf, err := bucket.NewConfig(engineBucketID, 1, nil)
	if err != nil {
		rel()
		return nil, err
	}
	if _, err := bucket.ExApplyBucketConfig(ctx, b, bucket.NewApplyBucketConfigToVolume(bucketConf, vol.GetID())); err != nil {
		rel()
		return nil, err
	}

	// Recover a missing persisted head instead of failing startup.
	engConf.DisableLookup = true
	engConf.RecoverMissingPersistedHead = true

	// Start the shared project World engine.
	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		b,
		engConf,
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels.add(worldCtrlRef.Release)

	// Resolve the engine and build its world state handle.
	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		rel()
		return nil, err
	}
	worldState := world.NewEngineWorldState(eng, true)

	// register the world operation types for manifests
	lookupOpCtrl := world.NewLookupOpController("bldr-manifest-ops", engineID, bldr_manifest_world.LookupOp)
	relLookupCtrl, err := b.AddController(ctx, lookupOpCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels.add(relLookupCtrl)

	// A distribution reuses only its own plugin cache. An earlier installation's
	// higher artifact revision cannot displace this build's embedded manifests.
	pluginHostObjectKey, err := bldr_dist.PluginHostObjectKey(distMeta)
	if err != nil {
		rel()
		return nil, err
	}
	if _, err := bldr_manifest_world.CreateManifestStoreInEngine(ctx, eng, pluginHostObjectKey); err != nil {
		rel()
		return nil, err
	}

	// The first manifest selection must see every resolver the config set
	// provides, such as a Release World announcing a newer release.
	select {
	case <-ctx.Done():
		rel()
		return nil, context.Cause(ctx)
	case <-configSetAttached:
	}

	// build the plugin scheduler
	pluginSchedConf := newReleaseSchedulerConfig(
		projectID,
		engineID,
		pluginHostObjectKey,
		vol.GetID(),
		vol.GetPeerID().String(),
	)
	pluginSchedConf.UpdateGuardPluginIds = slices.Clone(distMeta.GetUpdateGuardPluginIds())

	// Startup plugins ship with the distribution, so they may run their own
	// plugins, such as a Space's, on the distribution's plugin hosts.
	pluginSchedConf.HostExportPluginIds = slices.Clone(distMeta.GetStartupPlugins())
	pluginSchedCtrl, _, pluginSchedCtrlRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_scheduler.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(pluginSchedConf),
		nil,
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels.add(pluginSchedCtrlRef.Release)
	startupGroup := plugin_entrypoint_controller.NewStartupGroupCoordinator(
		distMeta.GetStartupPlugins(),
		pluginSchedCtrl,
	)
	pluginSchedCtrl.SetManifestCopyGate(startupGroup)

	// build the plugin host controller
	pluginHostCtrl, pluginHostRel, err := plugin_host_default.StartPluginHost(
		ctx,
		b,
		pluginsStateRoot,
		pluginsDistRoot,
		webRuntimeID,
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels.add(pluginHostRel)

	// Create LoadPlugin directives for the startup plugins.
	for _, pluginID := range distMeta.GetStartupPlugins() {
		_, pluginRef, err := b.AddDirective(bldr_plugin.NewLoadPlugin(pluginID), nil)
		if err != nil {
			le.WithError(err).WithField("plugin-id", pluginID).Warn("failed to load startup plugin")
			continue
		}
		rels.add(pluginRef.Release)
	}

	// Start the startup group after its plugins are loading.
	if err := startupGroup.Start(ctx); err != nil {
		rel()
		return nil, err
	}

	// Record the engine and bucket identifiers on the DistBus.
	distBus.worldEngineID = engineID
	distBus.engineBucketID = engineBucketID
	distBus.engineObjectStoreID = engineObjStoreID
	distBus.pluginHostObjectKey = pluginHostObjectKey

	// Record the plugin host and volume state on the DistBus.
	distBus.pluginSchedCtrl = pluginSchedCtrl
	distBus.pluginHostCtrl = pluginHostCtrl
	distBus.vol = vol
	distBus.peerID = vol.GetPeerID()
	distBus.worldEngine = eng
	distBus.worldState = worldState
	return distBus, nil
}

// applyConfigSet applies configSet to b until release is called. The returned
// channel closes once every controller in the set is constructed or has
// failed, so the directive resolvers of the constructed controllers are
// attached to b.
func applyConfigSet(b bus.Bus, configSet configset.ConfigSet) (attached <-chan struct{}, release func(), err error) {
	// An empty set has nothing to attach.
	done := make(chan struct{})
	if len(configSet) == 0 {
		close(done)
		return done, func() {}, nil
	}

	// Track the controllers that have not yet reached a final state.
	var mtx sync.Mutex
	pending := make(map[string]struct{}, len(configSet))
	for id := range configSet {
		pending[id] = struct{}{}
	}

	// Settle each controller on its first constructed or failed state.
	settle := func(val directive.AttachedValue) {
		// Ignore states still loading their controller.
		st, ok := val.GetValue().(configset.ApplyConfigSetValue)
		if !ok || st == nil || (st.GetController() == nil && st.GetError() == nil) {
			return
		}

		// Close done when the last pending controller settles.
		mtx.Lock()
		defer mtx.Unlock()
		if _, ok := pending[st.GetId()]; !ok {
			return
		}
		delete(pending, st.GetId())
		if len(pending) == 0 {
			close(done)
		}
	}
	_, ref, err := b.AddDirective(configset.NewApplyConfigSet(configSet), bus.NewCallbackHandler(settle, nil, nil))
	if err != nil {
		return nil, nil, err
	}
	return done, ref.Release, nil
}

// newReleaseSchedulerConfig copies remote manifests after startup for offline use.
func newReleaseSchedulerConfig(
	projectID,
	engineID,
	pluginHostObjectKey,
	volID,
	peerID string,
) *plugin_host_scheduler.Config {
	pluginSchedConf := plugin_host_default.NewSchedulerConfig(
		"",
		engineID,
		pluginHostObjectKey,
		volID,
		peerID,
		true,  // Watch FetchManifest on the bus so we can do auto-update via plugins.
		false, // Enable storing the manifest root in the plugin host world.

		// Dynamic providers can exit before a dependent plugin restarts, so
		// their manifest contents still need a complete local copy.
		false,
	)
	// The embedded distribution is already covered by the offline asset cache.
	// Release World manifests load on demand, then the startup-group gate lets
	// the scheduler copy their complete DAGs for offline restarts.
	pluginSchedConf.NoCopyBucketIds = []string{
		bldr_dist.GetDistBucketID(projectID),
	}
	// Startup waits for the announced release, so a returning visitor boots
	// the newest plugins once instead of booting the cached ones and replacing
	// them. An unreachable Release World leaves the cached release selectable.
	pluginSchedConf.AwaitFetchManifest = true
	return pluginSchedConf
}

// isWebDistPlatform reports whether the distribution uses browser storage.
func isWebDistPlatform(platformID string) bool {
	platform, err := bldr_platform.ParsePlatform(platformID)
	return err == nil && bldr_platform.IsWebPlatform(platform)
}

// GetContext returns the context.
func (d *DistBus) GetContext() context.Context {
	return d.ctx
}

// GetBus returns the bus.
func (d *DistBus) GetBus() bus.Bus {
	return d.b
}

// GetLogger returns the root logger.
func (d *DistBus) GetLogger() *logrus.Entry {
	return d.le
}

// GetStaticResolver returns the static controller resolver.
func (d *DistBus) GetStaticResolver() *static.Resolver {
	return d.sr
}

// GetStorageID returns the storage id.
func (d *DistBus) GetStorageID() string {
	return d.storageID
}

// GetDistPlatformID returns the distribution platform id.
func (d *DistBus) GetDistPlatformID() string {
	return d.platformID
}

// GetStateRoot returns the root of the state tree.
func (d *DistBus) GetStateRoot() string {
	return d.stateRoot
}

// GetVolume returns the storage volume in use.
func (d *DistBus) GetVolume() volume.Volume {
	return d.vol
}

// GetWorldEngineID returns the world engine id.
func (d *DistBus) GetWorldEngineID() string {
	return d.worldEngineID
}

// GetWorldEngine returns the world engine instance.
func (d *DistBus) GetWorldEngine() world.Engine {
	return d.worldEngine
}

// GetWorldState returns the world state handle.
func (d *DistBus) GetWorldState() world.WorldState {
	return d.worldState
}

// GetPluginHostObjectKey returns the object key for the plugin host.
func (d *DistBus) GetPluginHostObjectKey() string {
	return d.pluginHostObjectKey
}

// AddRelease registers cleanup to run after the bus-owned resources.
// Callers register cleanup before Release begins.
func (d *DistBus) AddRelease(release func()) {
	d.addRelease(release)
}

// Release releases the devtool bus.
func (d *DistBus) Release() {
	d.rel()
}
