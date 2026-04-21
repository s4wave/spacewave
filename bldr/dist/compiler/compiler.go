//go:build !js

package bldr_dist_compiler

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/directive"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/blang/semver/v4"
	pkgerrors "github.com/pkg/errors"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// ControllerID is the compiler controller ID.
const ControllerID = ConfigID

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
type PreBuildHook func(ctx context.Context, builderConf *bldr_manifest_builder.BuilderConfig, worldEng world.Engine) (*PreBuildHookResult, error)

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

// SupportsStartupManifestCache returns true if startup cache reuse is safe.
func (c *Controller) SupportsStartupManifestCache() bool {
	return false
}

// BuildManifest compiles the manifest once with the given builder args.
func (c *Controller) BuildManifest(
	ctx context.Context,
	args *bldr_manifest_builder.BuildManifestArgs,
	host bldr_manifest_builder.BuildManifestHost,
) (*bldr_manifest_builder.BuilderResult, error) {
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

	// clone the config and apply the pre-build hooks
	conf := c.GetConfig().CloneVT()

	// call any pre-build hooks
	for _, hook := range c.preBuildHooks {
		res, err := hook(ctx, builderConf, busEngine)
		if err != nil {
			return nil, err
		}

		conf.Merge(res.GetConfig())
	}

	// build base config sets
	hostConfigSet := make(map[string]*configset_proto.ControllerConfig, len(conf.GetHostConfigSet()))
	for k, v := range conf.GetHostConfigSet() {
		hostConfigSet[k] = v.CloneVT()
	}

	// build list of embed manifests & load plugins
	embedSpecs := slices.Clone(conf.GetEmbedManifests())
	embedManifestIDs := make([]string, len(embedSpecs))
	for i, em := range embedSpecs {
		embedManifestIDs[i] = em.GetManifestId()
	}
	loadPlugins := slices.Clone(conf.GetLoadPlugins())

	// determine project id
	projectID := builderConf.GetProjectId()
	if cproj := conf.GetProjectId(); cproj != "" {
		projectID = cproj
	}

	// sort and cleanup the fields
	conf.Normalize()

	le.Debug("compiling dist")
	entrypointFilename := projectID + buildPlatform.GetExecutableExt()
	manifestStoreObjKey := "dist"
	manifestStorePrefix := manifestStoreObjKey + "/"
	distMeta := bldr_dist.NewDistMeta(projectID, platformID, loadPlugins, nil, manifestStoreObjKey)

	searchKeys := builderConf.GetLinkObjectKeys()
	if len(searchKeys) == 0 {
		return nil, errors.New("link_object_keys is empty, cannot scan for manifests")
	}

	// Scope the manifest search to exactly the platform IDs referenced by
	// embed_manifests entries. Each embed names a specific (manifest_id,
	// platform_id) build; there is no implicit cross-platform resolution.
	searchPlatformSet := make(map[string]struct{}, len(embedSpecs))
	for _, em := range embedSpecs {
		searchPlatformSet[em.GetPlatformId()] = struct{}{}
	}
	searchPlatformIDs := make([]string, 0, len(searchPlatformSet))
	for p := range searchPlatformSet {
		searchPlatformIDs = append(searchPlatformIDs, p)
	}
	slices.Sort(searchPlatformIDs)

	// Wait for all requested (manifest_id, platform_id) builds to exist.
	embedManifests := make([]*bldr_manifest_world.CollectedManifest, len(embedSpecs))
	handler := world_control.NewWaitForStateHandler(func(
		ctx context.Context,
		ws world.WorldState,
		obj world.ObjectState,
		rootCs *block.Cursor,
		rev uint64,
	) (bool, error) {
		// Scan for manifests we want to embed. If no embeds are configured,
		// skip the scan entirely; there is nothing to wait for.
		if len(embedSpecs) == 0 {
			return false, nil
		}

		collectedManifests, manifestErrs, err := bldr_manifest_world.CollectManifests(ctx, ws, searchPlatformIDs, searchKeys...)
		if err != nil {
			return false, err
		}
		for _, err := range manifestErrs {
			le.WithError(err).Warn("skipped invalid manifest")
		}

		var notFoundDescs []string
		for i, em := range embedSpecs {
			// note: matchingManifests is sorted by rev, higher is first in the list.
			matchingManifests := collectedManifests[em.GetManifestId()]
			var found *bldr_manifest_world.CollectedManifest
			for _, cm := range matchingManifests {
				if cm.Manifest.GetMeta().GetPlatformId() == em.GetPlatformId() {
					found = cm
					break
				}
			}
			if found == nil {
				notFoundDescs = append(notFoundDescs, em.GetManifestId()+"@"+em.GetPlatformId())
			} else {
				embedManifests[i] = found
			}
		}

		// Wait for missing manifests to exist, if any.
		if len(notFoundDescs) != 0 {
			le.Infof("waiting for %d not-found embed manifests: %v", len(notFoundDescs), notFoundDescs)
			return true, nil
		}

		return false, nil
	})

	// Fan out one FetchManifest directive per embed tuple in parallel with the
	// world-scan watch loop. The watch loop populates CollectedManifest entries
	// as builds complete; the directives surface terminal builder errors so a
	// failed embed aborts the dist compile instead of hanging forever. First
	// error wins and cancels siblings.
	embedCtx, embedCancel := context.WithCancelCause(ctx)
	defer embedCancel(nil)

	var directiveRefs []directive.Reference
	var refsMu sync.Mutex
	defer func() {
		refsMu.Lock()
		for _, ref := range directiveRefs {
			if ref != nil {
				ref.Release()
			}
		}
		refsMu.Unlock()
	}()

	var embedWG sync.WaitGroup
	for _, em := range embedSpecs {
		embedWG.Go(func() {
			dir := bldr_manifest.NewFetchManifest(em.GetManifestId(), nil, []string{em.GetPlatformId()}, 0)
			_, _, ref, err := bus.ExecWaitValue[*bldr_manifest.FetchManifestValue](
				embedCtx,
				c.GetBus(),
				dir,
				func(isIdle bool, errs []error) (bool, error) {
					if isIdle && len(errs) != 0 {
						return false, errs[0]
					}
					return true, nil
				},
				nil,
				nil,
			)
			if err != nil {
				if embedCtx.Err() == nil {
					embedCancel(pkgerrors.Wrapf(err, "embed %s@%s", em.GetManifestId(), em.GetPlatformId()))
				}
				return
			}
			if ref != nil {
				refsMu.Lock()
				directiveRefs = append(directiveRefs, ref)
				refsMu.Unlock()
			}
		})
	}

	// use short-lived read transactions
	watchLoop := world_control.NewWatchLoop(le, "", handler)
	ws := world.NewEngineWorldState(busEngine, false)
	watchErr := watchLoop.Execute(embedCtx, ws)

	// Unblock any in-flight FetchManifest directives before returning.
	embedCancel(nil)
	embedWG.Wait()

	if watchErr != nil {
		if cause := context.Cause(embedCtx); cause != nil && cause != context.Canceled && !errors.Is(cause, context.Canceled) {
			return nil, cause
		}
		return nil, watchErr
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
				buildTimestamp.CloneVT(),
			)
			if err != nil {
				embedTx.Discard()
				return err
			}
			if err := embedTx.Commit(ctx); err != nil {
				return err
			}
		}

		return nil
	}

	webStartupSrcPath, err := conf.ParseWebStartupPath()
	if err != nil {
		return nil, err
	}

	err = BuildDistBundle(
		ctx,
		le,
		builderConf.GetSourcePath(),
		builderConf.GetDistSourcePath(),
		webStartupSrcPath,
		workingPath,
		outDistPath,
		entrypointFilename,
		distMeta,
		buildType,
		buildPlatform,
		hostConfigSet,
		initEmbeddedWorld,
		conf.GetEnableCgo(),
		conf.GetEnableTinygo(),
		conf.GetEnableCompression(),
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
	result := bldr_manifest_builder.NewBuilderResult(
		committedManifest,
		committedManifestRef,
		bldr_manifest_builder.NewInputManifest(nil, nil),
	)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return result, nil
}

// GetSupportedPlatforms returns the base platform IDs this compiler supports.
// The dist compiler supports native and web platforms including WebAssembly.
func (c *Controller) GetSupportedPlatforms() []string {
	return []string{bldr_platform.PlatformID_DESKTOP, bldr_platform.PlatformID_WEB}
}

// _ is a type assertion
var _ bldr_manifest_builder.Controller = ((*Controller)(nil))
