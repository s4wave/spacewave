package bldr_dist_compiler

import (
	"context"
	"errors"
	"path/filepath"
	"sort"

	"github.com/aperturerobotics/bifrost/peer"
	bldr_dist "github.com/aperturerobotics/bldr/dist"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/aperturerobotics/timestamp"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

// ControllerID is the compiler controller ID.
const ControllerID = "bldr/dist/compiler"

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "dist compiler controller"

// Controller is the compiler controller.
type Controller struct {
	*bus.BusController[*Config]
	preBuildHooks []PreBuildHook
}

// Factory is the factory for the compiler controller.
type Factory = bus.BusFactory[*Config, *Controller]

// NewController constructs a new dist compiler controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) (*Controller, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return &Controller{
		BusController: bus.NewBusController(
			le,
			b,
			conf,
			ControllerID,
			Version,
			controllerDescrip,
		),
	}, nil
}

// NewFactory constructs a new dist compiler controller factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		NewConfig,
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{
				BusController: base,
			}, nil
		},
	)
}

// PreBuildHook is a callback called before building the dist.
// Returns an optional PreBuildResult.
type PreBuildHook func(ctx context.Context, builderConf *manifest_builder.BuilderConfig, worldEng world.Engine) (*PreBuildHookResult, error)

// AddPreBuildHook adds a callback that is called just after constructing the dist working dir.
// Called before calling the Go compiler or bundling the assets or dist fs.
// NOTE: may be removed in future
func (c *Controller) AddPreBuildHook(hook PreBuildHook) {
	if hook != nil {
		c.preBuildHooks = append(c.preBuildHooks, hook)
	}
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// BuildManifest compiles the manifest once with the given builder args.
func (c *Controller) BuildManifest(
	ctx context.Context,
	args *manifest_builder.BuildManifestArgs,
) (*manifest_builder.BuilderResult, error) {
	conf := c.GetConfig()
	builderConf := args.GetBuilderConfig()
	meta, buildPlatform, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}

	platformID := meta.GetPlatformId()
	manifestID := meta.GetManifestId()
	buildType := bldr_manifest.ToBuildType(meta.GetBuildType())
	buildTimestamp := timestamp.Now()

	le := c.GetLogger().
		WithField("manifest-id", manifestID).
		WithField("build-type", buildType).
		WithField("platform-id", platformID)
	le.Debug("building dist manifest")

	// clean / create dist dir
	outDistPath := filepath.Join(builderConf.GetWorkingPath(), "dist")
	if err := fsutil.CleanCreateDir(outDistPath); err != nil {
		return nil, err
	}

	// clean / create assets dir
	outAssetsPath := filepath.Join(builderConf.GetWorkingPath(), "assets")
	if err := fsutil.CleanCreateDir(outAssetsPath); err != nil {
		return nil, err
	}

	// working path
	workingPath := builderConf.GetWorkingPath()

	// build output world engine
	busEngine := world.NewBusEngine(ctx, c.GetBus(), builderConf.GetEngineId())

	// build base config sets
	hostConfigSet := make(map[string]*configset_proto.ControllerConfig, len(conf.GetHostConfigSet()))
	for k, v := range conf.GetHostConfigSet() {
		hostConfigSet[k] = v.CloneVT()
	}

	// build list of embed manifests & load plugins
	embedManifestIDs := slices.Clone(conf.GetEmbedManifests())
	loadPlugins := slices.Clone(conf.GetLoadPlugins())

	// determine project id
	projectID := builderConf.GetProjectId()
	if cproj := conf.GetProjectId(); cproj != "" {
		projectID = cproj
	}

	// call any pre-build hooks
	for _, hook := range c.preBuildHooks {
		res, err := hook(ctx, builderConf, busEngine)
		if err != nil {
			return nil, err
		}

		// merge config sets
		resHostConfigSet := res.GetHostConfigSet()
		if len(resHostConfigSet) != 0 {
			configset_proto.MergeConfigSetMaps(hostConfigSet, resHostConfigSet)
		}

		// append embed manifests list and load plugins list
		embedManifestIDs = append(embedManifestIDs, res.GetEmbedManifests()...)
		loadPlugins = append(loadPlugins, res.GetLoadPlugins()...)

		// override project id
		if cproj := res.GetProjectId(); cproj != "" {
			projectID = cproj
		}
	}

	// Cleanup lists
	sort.Strings(embedManifestIDs)
	embedManifestIDs = slices.Compact(embedManifestIDs)
	sort.Strings(loadPlugins)
	loadPlugins = slices.Compact(loadPlugins)

	le.Debug("compiling dist")
	entrypointFilename := projectID + buildPlatform.GetExecutableExt()
	manifestStoreObjKey := "dist"
	manifestStorePrefix := manifestStoreObjKey + "/"
	distMeta := bldr_dist.NewDistMeta(projectID, platformID, loadPlugins, nil, manifestStoreObjKey)

	searchKeys := builderConf.GetLinkObjectKeys()
	if len(searchKeys) == 0 {
		return nil, errors.New("link_object_keys is empty, cannot scan for manifests")
	}

	// Wait for all manifests to exist.
	embedManifests := make([]*bldr_manifest_world.CollectedManifest, len(embedManifestIDs))
	handler := world_control.NewWaitForStateHandler(func(
		ctx context.Context,
		ws world.WorldState,
		obj world.ObjectState,
		rootCs *block.Cursor,
		rev uint64,
	) (bool, error) {
		// Scan for manifests we want to embed.
		collectedManifests, manifestErrs, err := bldr_manifest_world.CollectManifests(ctx, ws, platformID, searchKeys...)
		if err != nil {
			return false, err
		}
		for _, err := range manifestErrs {
			le.WithError(err).Warn("skipped invalid manifest")
		}

		var notFoundManifestIDs []string
		for i, embedManifestID := range embedManifestIDs {
			// note: matchingManifests is sorted by rev, higher is first in the list.
			matchingManifests := collectedManifests[embedManifestID]
			if len(matchingManifests) == 0 {
				notFoundManifestIDs = append(notFoundManifestIDs, embedManifestID)
				// return errors.Wrap(bldr_manifest.ErrNotFoundManifest, embedManifestID)
			} else {
				embedManifests[i] = matchingManifests[0]
			}
		}

		// Wait for missing manifests to exist, if any.
		if len(notFoundManifestIDs) != 0 {
			le.Infof("waiting for %d not-found manifests: %v", len(notFoundManifestIDs), notFoundManifestIDs)
			return true, nil
		}

		return false, nil
	})

	// use short-lived read transactions
	watchLoop := world_control.NewWatchLoop(le, "", handler)
	ws := world.NewEngineWorldState(busEngine, false)
	if err := watchLoop.Execute(ctx, ws); err != nil {
		return nil, err
	}

	// When we compile the bundle we will copy the embed manifests to the embed volume.
	initEmbeddedWorld := func(ctx context.Context, embedEngine world.Engine, embedOpPeerID peer.ID) error {
		// Create the base object store.
		le.
			WithField("manifest-store-id", manifestStoreObjKey).
			Debug("creating manifest store")
		if _, err := bldr_manifest_world.CreateManifestStoreInEngine(ctx, embedEngine, manifestStoreObjKey); err != nil {
			return err
		}

		// Copy the embed plugin manifests to the embedded manifests world.
		// embedManifestsObjKeys := make([]string, 0, len(embedManifests))
		for _, embedManifestInfo := range embedManifests {
			le.
				WithField("copy-manifest-id", embedManifestInfo.Manifest.GetMeta().GetManifestId()).
				WithField("copy-manifest-rev", embedManifestInfo.Manifest.GetMeta().GetRev()).
				Debug("copying manifest to embedded volume")
			embedTx, err := embedEngine.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			manifestObjKey := manifestStorePrefix + embedManifestInfo.Manifest.GetMeta().GetManifestId()
			_, _, err = bldr_manifest_world.DeepCopyManifest(
				ctx,
				le,
				ws.AccessWorldState,
				embedManifestInfo.ManifestRef,
				embedTx,
				embedTx.AccessWorldState,
				manifestObjKey,
				[]string{manifestStoreObjKey},
				embedOpPeerID,
				buildTimestamp.Clone(),
			)
			if err != nil {
				embedTx.Discard()
				return err
			}
			// embedManifestsObjKeys = append(embedManifestsObjKeys, manifestObjKey)
			if err := embedTx.Commit(ctx); err != nil {
				return err
			}
		}

		// cleanup embedManifestsObjKeys
		// sort.Strings(embedManifestsObjKeys)
		// embedManifestsObjKeys = slices.Compact(embedManifestsObjKeys)
		return nil
	}

	err = BuildDistBundle(
		ctx,
		le,
		builderConf.GetDistSourcePath(),
		workingPath,
		outDistPath,
		entrypointFilename,
		distMeta,
		buildType,
		buildPlatform,
		hostConfigSet,
		initEmbeddedWorld,
		conf.GetEnableCgo(),
	)
	if err != nil {
		return nil, err
	}

	tx, err := busEngine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	le.Debug("bundling dist files")
	// bundle dist and assets fs
	committedManifest, committedManifestRef, err := builderConf.CommitManifestWithPaths(
		ctx,
		le,
		tx,
		meta,
		entrypointFilename,
		outDistPath,
		outAssetsPath,
	)
	if err != nil {
		return nil, err
	}

	le.Debug("dist build complete")
	result := manifest_builder.NewBuilderResult(
		committedManifest,
		committedManifestRef,
		manifest_builder.NewInputManifest(nil),
	)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return result, nil
}

// _ is a type assertion
var _ manifest_builder.Controller = ((*Controller)(nil))
