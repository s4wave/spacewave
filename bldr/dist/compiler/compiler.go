//go:build !js

package bldr_dist_compiler

import (
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/util/fsutil"
	pkgerrors "github.com/pkg/errors"
	bldr_cli_compiler "github.com/s4wave/spacewave/bldr/cli/compiler"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_manifest_pack "github.com/s4wave/spacewave/bldr/manifest/pack"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
)

// ControllerID is the compiler controller ID.
const ControllerID = ConfigID

// Version is the controller version.
var Version = controller.MustParseVersion("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "dist compiler controller"

// Controller is the compiler controller.
type Controller struct {
	*bus.BusController[*Config]
	// preBuildHooks configure the distribution before its manifests are resolved.
	preBuildHooks []PreBuildHook
}

// Factory is the factory for the compiler controller.
type Factory = bus.BusFactory[*Config, *Controller]

// NewController constructs a new dist compiler controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) (*Controller, error) {
	// Validate configuration before constructing the compiler.
	if err := conf.Validate(); err != nil {
		return nil, err
	}

	// Attach the configured bus and controller identity.
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
// Returns an optional PreBuildHookResult.
type PreBuildHook func(ctx context.Context, builderConf *bldr_manifest_builder.BuilderConfig, worldEng world.Engine) (*PreBuildHookResult, error)

// AddPreBuildHook adds a callback that is called just after constructing the
// dist working dir. Called before calling the Go compiler or bundling the
// assets or dist fs.
//
// Hooks must be registered before BuildManifest starts.
func (c *Controller) AddPreBuildHook(hook PreBuildHook) {
	if hook != nil {
		c.preBuildHooks = append(c.preBuildHooks, hook)
	}
}

// Execute implements controller.Controller.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// SupportsStartupManifestCache returns false: the dist compiler cannot safely
// reuse a startup manifest cache.
func (c *Controller) SupportsStartupManifestCache() bool {
	return false
}

// BuildManifest compiles the manifest once with the given builder args.
//
// BuildManifest is the main pipeline for the dist compiler: resolve metadata,
// clean output dirs, load the source module, run pre-build hooks, collect and
// copy the embed manifests, bundle the dist, and commit the result.
func (c *Controller) BuildManifest(
	ctx context.Context,
	args *bldr_manifest_builder.BuildManifestArgs,
	host bldr_manifest_builder.BuildManifestHost,
) (*bldr_manifest_builder.BuilderResult, error) {
	// Resolve the requested build identity and target platform.
	builderConf := args.GetBuilderConfig()
	meta, buildPlatform, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}

	// Scope build diagnostics to the resolved manifest.
	platformID := meta.GetPlatformId()
	manifestID := meta.GetManifestId()
	buildType := bldr_manifest.ToBuildType(meta.GetBuildType())
	buildTimestamp := bldr_manifest_builder.ManifestCommitTimestamp(ctx)
	le := c.GetLogger().
		WithField("manifest-id", manifestID).
		WithField("build-type", buildType).
		WithField("platform-id", platformID)
	le.Debug("building dist manifest")

	// Clean and create the dist dir.
	outDistPath := filepath.Join(builderConf.GetWorkingPath(), "dist")
	if err := fsutil.CleanCreateDir(outDistPath); err != nil {
		return nil, err
	}

	// Clean and create the assets dir.
	outAssetsPath := filepath.Join(builderConf.GetWorkingPath(), "assets")
	if err := fsutil.CleanCreateDir(outAssetsPath); err != nil {
		return nil, err
	}

	// Resolve the working and source paths.
	workingPath := builderConf.GetWorkingPath()
	sourcePath := builderConf.GetSourcePath()
	goModPath := filepath.Join(sourcePath, "go.mod")
	goModData, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, err
	}
	rootModule := modfile.ModulePath(goModData)

	// Build the output world engine.
	busEngine := world.NewBusEngine(ctx, c.GetBus(), builderConf.GetEngineId())

	// Apply pre-build hooks to a private configuration snapshot.
	conf := c.GetConfig().CloneVT()
	for _, hook := range c.preBuildHooks {
		res, err := hook(ctx, builderConf, busEngine)
		if err != nil {
			return nil, err
		}
		conf.Merge(res.GetConfig())
	}

	// Build the base config sets.
	hostConfigSet := make(map[string]*configset_proto.ControllerConfig, len(conf.GetHostConfigSet()))
	for k, v := range conf.GetHostConfigSet() {
		hostConfigSet[k] = v.CloneVT()
	}

	// Build the list of embed manifests and load plugins.
	embedSpecs := slices.Clone(conf.GetEmbedManifests())
	loadPlugins := slices.Clone(conf.GetLoadPlugins())

	// Determine the project id.
	projectID := builderConf.GetProjectId()
	if cproj := conf.GetProjectId(); cproj != "" {
		projectID = cproj
	}
	entrypointRole := conf.GetEntrypointRole()
	if entrypointRole == "" {
		entrypointRole = bldr_dist.EntrypointRoleDesktop
	}
	channelKey := conf.GetChannelKey()
	if channelKey == "" {
		channelKey = "stable"
	}

	// Sort and clean up the fields.
	conf.Normalize()

	// Describe the installed entrypoint and its embedded manifest store.
	le.Debug("compiling dist")
	entrypointFilename := projectID + buildPlatform.GetExecutableExt()
	manifestStoreObjKey := "dist"
	manifestStorePrefix := manifestStoreObjKey + "/"
	distMeta := bldr_dist.NewDistEntrypointMeta(
		projectID,
		platformID,
		loadPlugins,
		nil,
		manifestStoreObjKey,
		entrypointRole,
		channelKey,
		manifestID,
		meta.GetRev(),
	)

	// FetchManifest owns build-type and platform selection, readiness, and errors.
	// Resolve each immutable reference before copying its DAG into the bundle.
	embedManifests := make([]*bldr_manifest.ManifestRef, len(embedSpecs))
	for i, em := range embedSpecs {
		ref, err := bldr_manifest_pack.ResolveManifestTuple(ctx, c.GetBus(), &bldr_manifest_pack.ManifestTuple{
			ManifestId: em.GetManifestId(),
			PlatformId: em.GetPlatformId(),
			ObjectKey:  manifestStorePrefix + em.GetManifestId(),
		}, buildType.String())
		if err != nil {
			return nil, pkgerrors.Wrapf(err, "embed %s@%s", em.GetManifestId(), em.GetPlatformId())
		}
		embedManifests[i] = ref
	}
	ws := world.NewEngineWorldState(busEngine, false)

	// The bundle callback copies exactly the references selected by FetchManifest.
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
			// Isolate each manifest copy in a transaction on the embedded volume.
			le.
				WithField("copy-manifest-id", embedManifestInfo.GetMeta().GetManifestId()).
				WithField("copy-manifest-rev", embedManifestInfo.GetMeta().GetRev()).
				Debug("copying manifest to embedded volume")
			embedTx, err := embedEngine.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			defer embedTx.Discard()

			// Preserve the fetched manifest DAG and attach it to the embedded store.
			manifestObjKey := manifestStorePrefix + embedManifestInfo.GetMeta().GetManifestId()
			_, _, err = bldr_manifest_world.DeepCopyManifest(
				ctx,
				le,
				ws.AccessWorldState,
				embedManifestInfo.GetManifestRef(),
				nil,
				embedTx,
				embedTx.AccessWorldState,
				manifestObjKey,
				[]string{manifestStoreObjKey},
				embedOpPeerID,
				buildTimestamp.CloneVT(),
			)
			if err != nil {
				return err
			}
			if err := embedTx.Commit(ctx); err != nil {
				return err
			}
		}

		return nil
	}

	// Resolve the configured web entrypoint before constructing imports.
	webStartupSrcPath, err := conf.ParseWebStartupPath()
	if err != nil {
		return nil, err
	}

	// Native distributions include CLI packages for their target OS and architecture.
	var cliImports map[string]bldr_cli_compiler.CliImport
	if !bldr_platform.IsWebPlatform(buildPlatform) {
		cliPkgs, _ := plugin_compiler_go.UpdateRelativeGoPackagePaths(
			conf.GetCliPkgs(),
			rootModule,
		)
		var goos, goarch string
		if native, ok := buildPlatform.(*bldr_platform.NativePlatform); ok {
			goos, goarch = native.GetGOOS(), native.GetGOARCH()
		}
		cliImports, err = bldr_cli_compiler.AnalyzeCliImports(ctx, le, sourcePath, cliPkgs, goos, goarch)
		if err != nil {
			return nil, err
		}
	}

	// Compile the host and copy the resolved plugin DAGs into its embedded volume.
	err = BuildDistBundle(
		ctx,
		le,
		sourcePath,
		builderConf.GetDistSourcePath(),
		webStartupSrcPath,
		workingPath,
		outDistPath,
		entrypointFilename,
		distMeta,
		buildType,
		builderConf.GetBuildPolicy(),
		buildPlatform,
		hostConfigSet,
		initEmbeddedWorld,
		cliImports,
		conf.GetEnableCgo(),
		conf.GetGoCompiler(),
		conf.GetEnableCompression(),
		conf.GetBrowserIceServers(),
		conf.GetBrowserIceServersEndpoint(),
	)
	if err != nil {
		return nil, err
	}

	// Publish the complete distribution manifest in one transaction.
	tx, err := busEngine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Commit the executable and assets together.
	le.Debug("bundling dist files")
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

	// Return the committed distribution and its build provenance.
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

// _ verifies the manifest compiler contract.
var _ bldr_manifest_builder.Controller = (*Controller)(nil)
