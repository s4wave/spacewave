//go:build !js && !tinygo

package space_exec

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/filter"
	"github.com/aperturerobotics/util/pipesock"
	"github.com/pkg/errors"
	spacewave "github.com/s4wave/spacewave"
	"github.com/s4wave/spacewave/bldr"
	"github.com/s4wave/spacewave/bldr/devtool"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/s4wave/spacewave/bldr/manifest/builder/resultworld"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/bldr/plugin/compiler/esbuild"
	js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/bldr/project/bunlock"
	project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	validate "github.com/s4wave/spacewave/bldr/project/validate"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
	app_web "github.com/s4wave/spacewave/web"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

// RepositoryCompilerLoader constructs a build target on demand. The handler
// retains the source and dependency tree until the target finishes and closes.
type RepositoryCompilerLoader func(context.Context, *logrus.Entry, bus.Bus, *unixfs.FSHandle, kvtx.TxOps, *js.Config, int64) (builder.Controller, error)

// buildPluginHandler compiles a pinned Space source tree on its assigned native worker.
// Forge owns scheduling and cancellation; Bldr owns compilation and build provenance.
type buildPluginHandler struct {
	// le records compiler diagnostics in the enclosing execution.
	le *logrus.Entry
	// engine is the execution's accepted World capability.
	engine world.Engine
	// handle retains storage and the assigned worker's identity.
	handle forge_target.ExecControllerHandle
	// source is the exact input snapshot retained before compilation.
	source *forge_value.WorldObjectSnapshot
	// manifestID selects the output plugin in the source project.
	manifestID string
	// platformID is the platform the immutable build targets.
	platformID string
	// configPath identifies the project configuration inside source.
	configPath string
	// installer is the Session that submitted the build with the authority to
	// install plugins. It is empty when the build only retains its artifact.
	installer peer.ID
	// bus supplies the assigned device's authenticated transport.
	bus bus.Bus
	// config selects an immutable build or a retained frontend attachment.
	config *PluginBuildConfig
	// loadRepositoryCompiler constructs the JS repository target only during a build.
	loadRepositoryCompiler RepositoryCompilerLoader
}

// NewBuildPluginHandlerWithBus binds the build to the execution's World capability.
// The bus supplies the assigned device's network transport for live authoring.
func NewBuildPluginHandlerWithBus(ctx context.Context, b bus.Bus, le *logrus.Entry, ws world.WorldState, handle forge_target.ExecControllerHandle, inputs forge_target.InputMap, data []byte) (Handler, error) {
	return NewBuildPluginHandlerWithRepositoryCompiler(ctx, b, le, ws, handle, inputs, data, esbuild.NewRepositoryCompiler)
}

// NewBuildPluginHandlerWithRepositoryCompiler binds the handler with an on-demand
// repository target loader. Construction does not load the compiler or read source.
func NewBuildPluginHandlerWithRepositoryCompiler(_ context.Context, b bus.Bus, le *logrus.Entry, _ world.WorldState, handle forge_target.ExecControllerHandle, inputs forge_target.InputMap, data []byte, load RepositoryCompilerLoader) (Handler, error) {
	// Decode the request and validate its mode-specific fields.
	conf := &PluginBuildConfig{}
	if err := conf.UnmarshalJSON(data); err != nil {
		return nil, errors.Wrap(err, "parse plugin build config")
	}
	if err := validatePluginBuildMode(conf, b); err != nil {
		return nil, err
	}

	// Require the pinned Space directory the Task or Execution retained.
	inputSource, err := forge_target.InputValueToValue(inputs["source"])
	if err != nil {
		return nil, err
	}
	source := inputSource.GetWorldObjectSnapshot()
	if source.GetRootRef() == nil || source.GetObjectType() != unixfs_world.FSNodeTypeID {
		return nil, errors.New("plugin source must be a pinned Space UnixFS directory")
	}

	// Resolve the plugin identity and project configuration inside the source.
	manifestID := conf.GetManifestId()
	if err := manifest.ValidateManifestID(manifestID, false); err != nil {
		return nil, err
	}
	configPath := conf.GetConfigPath()
	if configPath == "" {
		configPath = "bldr.yaml"
	}
	if !fs.ValidPath(configPath) {
		return nil, errors.New("config_path must be relative to the source tree")
	}

	// Resolve the Session whose installation authority the submission checked.
	var installer peer.ID
	if conf.GetInstallerPeerId() != "" {
		installer, err = confparse.ParsePeerID(conf.GetInstallerPeerId())
		if err != nil {
			return nil, errors.Wrap(err, "parse plugin installer Session")
		}
	}

	// Require the transactional World the artifacts are written into.
	input, ok := inputs["world"].(forge_target.InputValueWorld)
	if !ok || input.GetWorldEngine() == nil {
		return nil, errors.New("plugin build requires a transactional World input")
	}
	return &buildPluginHandler{
		le: le, engine: input.GetWorldEngine(), handle: handle,
		source: source.CloneVT(), manifestID: manifestID, platformID: conf.GetPlatformId(),
		configPath: configPath, installer: installer, bus: b, config: conf,
		loadRepositoryCompiler: load,
	}, nil
}

// Execute retains exact source, then lets Bldr write artifacts into the supplied World.
// Repository builds read blocks; other sources use a disposable native checkout.
func (h *buildPluginHandler) Execute(ctx context.Context) error {
	// Build admitted JS repositories from blocks without starting native tooling.
	if h.config.GetFrontendId() == "" && h.platformID == bldr_platform.PlatformID_JS && strings.HasPrefix(h.source.GetKey(), "plugin-repos/") {
		return h.buildRepository(ctx)
	}

	// Create the disposable checkout directory.
	live := h.config.GetFrontendId() != ""
	working, err := os.MkdirTemp("", "spacewave-plugin-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(working)
	working, err = filepath.EvalSymlinks(working)
	if err != nil {
		return err
	}

	// Materialize the pinned source into the checkout.
	sourceRoot := filepath.Join(working, "source")
	stateRoot := filepath.Join(sourceRoot, ".bldr")
	if err := h.materializeSource(ctx, sourceRoot); err != nil {
		return errors.Wrap(err, "materialize plugin source")
	}
	if err := h.handle.WriteLog(ctx, "info", "Building "+h.manifestID+" from "+h.source.GetKey()); err != nil {
		return err
	}

	// Prepare the shipped TypeScript SDK without requiring a Go checkout in the Space.
	buildBus, err := devtool.BuildDevtoolBus(ctx, h.le, sourceRoot, stateRoot, live, nil)
	if err != nil {
		return err
	}
	defer buildBus.Release()
	if err := h.prepareSDK(ctx, sourceRoot, filepath.Join(stateRoot, "src")); err != nil {
		return err
	}

	// Compile directly into the accepted World, preserving child build results and DAG refs.
	const engineID = "plugin-build-space"
	releaseEngine, err := buildBus.GetBus().AddController(ctx, world.NewEngineController(engineID, h.engine), nil)
	if err != nil {
		return err
	}
	defer releaseEngine()

	// Start the project controller that owns the manifest builders.
	buildKey := path.Join(h.source.GetKey(), "builds")
	watcher, projectRef, err := buildBus.StartProjectControllerWithRemote(ctx, sourceRoot, h.configPath, "space", &project.RemoteConfig{
		EngineId:       engineID,
		PeerId:         h.handle.GetPeerId().String(),
		ObjectKey:      buildKey,
		LinkObjectKeys: []string{buildKey},
	}, h.config.GetFrontendRoutePrefix())
	if err != nil {
		return err
	}
	defer projectRef.Release()
	compiler, err := watcher.GetProjectController().WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// Serve a live frontend attachment instead of a one-shot build.
	if live {
		return h.runFrontend(ctx, sourceRoot, compiler.GetFrontendService())
	}

	// Build the requested platform's manifest and check its identity.
	refs, keys, err := compiler.BuildManifestBuilderConfigs(ctx, []*project_controller.ManifestBuilderConfig{
		project_controller.NewManifestBuilderConfig(h.manifestID, string(manifest.BuildType_DEV), h.platformID, "space"),
	})
	if err != nil {
		return err
	}
	if len(refs) != 1 || len(keys) != 1 {
		return errors.New("plugin compiler did not produce one manifest")
	}
	if got := refs[0].GetMeta().GetPlatformId(); got != h.platformID {
		return errors.Errorf("plugin compiler built platform %q, not %q", got, h.platformID)
	}

	return h.completeBuild(ctx, keys[0], refs[0], nil)
}

// completeBuild retains source and provenance, installs the artifact and reports outputs.
func (h *buildPluginHandler) completeBuild(ctx context.Context, key string, ref *manifest.ManifestRef, result *builder.BuilderResult) error {
	// Retain the source with the artifact and install under the submitting Session.
	resultRef, err := h.retainBuild(ctx, key, ref, result)
	if err != nil {
		return err
	}

	// Publish the immutable output identities to Forge.
	return h.handle.SetOutputs(ctx, forge_value.ValueSlice{
		forge_value.NewValueWithWorldObjectSnapshot("source", h.source),
		forge_value.NewValueWithWorldObjectSnapshot("manifest", &forge_value.WorldObjectSnapshot{
			Key: key, RootRef: ref.GetManifestRef(), ObjectType: "bldr/manifest",
		}),
		forge_value.NewValueWithBucketRef("build-result", resultRef),
	}, true)
}

// buildRepository reads an admitted source snapshot and fetches its verified pins
// into a build-scoped block tree. Only BuildSpacePlugin decides repository refusal.
func (h *buildPluginHandler) buildRepository(ctx context.Context) error {
	// Resolve the output World without starting the native project controller.
	engineID := "plugin-build-" + h.handle.GetExecutionUniqueId()
	releaseEngine, err := h.bus.AddController(ctx, world.NewEngineController(engineID, h.engine), nil)
	if err != nil {
		return err
	}
	defer releaseEngine()

	// Keep the snapshot cursor and dependency blocks live through compilation.
	var result *builder.BuilderResult
	err = h.handle.AccessStorage(ctx, h.source.GetRootRef(), func(cursor *bucket_lookup.Cursor) error {
		// Open the pinned directory directly through the execution's capability.
		filesystem := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, cursor.Clone(), nil)
		source, err := unixfs.NewFSHandle(filesystem)
		if err != nil {
			filesystem.Release()
			return err
		}
		defer source.Release()
		fsys := unixfs_iofs.NewFS(ctx, source)

		// Read the admitted plugin configuration from the same immutable snapshot.
		project, _, err := validate.ValidateProject(ctx, fsys)
		if err != nil {
			return err
		}
		selected := project.GetManifests()[h.manifestID]
		if selected == nil {
			return errors.Errorf("plugin repository does not declare plugin %q", h.manifestID)
		}

		// Apply the plugin's metadata and effective JS module configuration.
		plugin := &js.Config{}
		if err := plugin.UnmarshalJSON(selected.GetBuilder().GetConfig()); err != nil {
			return err
		}
		meta, platform, err := (&manifest.ManifestMeta{
			ManifestId: h.manifestID, PlatformId: h.platformID, BuildType: string(manifest.BuildType_DEV),
			Description: selected.GetDescription(), Background: selected.GetBackground(), Rev: selected.GetRev(),
			NodeTypeIds: selected.GetNodeTypeIds(),
		}).Resolve()
		if err != nil {
			return err
		}
		plugin.FlattenBuildTypes(manifest.BuildType_DEV)
		plugin.FlattenPlatformTypes(platform)

		// Open a writable dependency tree over the snapshot's effective block store.
		_, root := block.NewTransaction(cursor.GetBlockStore(), cursor.GetTransformer(), nil, nil)
		root.SetBlock(kvtx_block.NewKeyValueStore(0), true)
		dependencies, err := kvtx_block.BuildKvTransaction(ctx, root, true)
		if err != nil {
			return err
		}
		defer dependencies.Discard()

		// Fetch and commit the integrity pins before exposing them to the compiler.
		fetcher, err := bunlock.NewFetcher(http.DefaultClient, "https://registry.npmjs.org", 64<<20)
		if err != nil {
			return err
		}
		lock, err := fs.ReadFile(fsys, "bun.lock")
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(lock) != 0 {
			if _, err := fetcher.Fetch(ctx, lock, dependencies); err != nil {
				return err
			}
		}
		if err := dependencies.Commit(ctx); err != nil {
			return err
		}

		// Load and close the compiler only for this requested build.
		target, err := h.loadRepositoryCompiler(ctx, h.le, h.bus, source, dependencies, plugin, 256<<20)
		if err != nil {
			return err
		}
		defer target.Close()
		if err := h.handle.WriteLog(ctx, "info", "Building "+h.manifestID+" from "+h.source.GetKey()); err != nil {
			return err
		}
		result, err = target.BuildManifest(ctx, &builder.BuildManifestArgs{BuilderConfig: &builder.BuilderConfig{
			ManifestMeta: meta, EngineId: engineID, PeerId: h.handle.GetPeerId().String(),
			ProjectId: project.GetId(), LinkObjectKeys: []string{h.source.GetKey()},
		}}, nil)
		return err
	})
	if err != nil {
		return err
	}

	// Store compiler provenance with the source and installation in one transaction.
	if err := result.Validate(); err != nil {
		return err
	}
	ref := result.GetManifestRef()
	if ref.GetMeta().GetPlatformId() != h.platformID {
		return errors.New("repository compiler returned a different platform")
	}
	return h.completeBuild(ctx, manifest.NewManifestArtifactKey(ref.GetManifestRef()), ref, result)
}

// prepareSDK writes the shipped TypeScript SDK under distRoot for the build.
func (h *buildPluginHandler) prepareSDK(ctx context.Context, sourceRoot, distRoot string) error {
	// Sync the SDK and the dependency sources it imports by their vendored paths.
	const vendorRoot = "vendor/github.com/aperturerobotics/"
	if err := bldr.PrepareTypeScriptProject(ctx, h.le, sourceRoot, distRoot, map[string]fs.FS{
		".":    spacewave.DistSources,
		"bldr": bldr.DistSources,
		"web":  app_web.DistSources,

		// The SDK imports these dependency sources by their vendored paths.
		vendorRoot + "controllerbus/controller":                 controller.DistSources,
		vendorRoot + "controllerbus/controller/configset/proto": configset_proto.DistSources,
		vendorRoot + "controllerbus/controller/exec":            controller_exec.DistSources,
		vendorRoot + "starpc/rpcstream":                         rpcstream.DistSources,
		vendorRoot + "util/backoff":                             backoff.DistSources,
		vendorRoot + "util/csync":                               csync.DistSources,
		vendorRoot + "util/filter":                              filter.DistSources,
		vendorRoot + "util/pipesock":                            pipesock.DistSources,
	}); err != nil {
		return err
	}

	// Resolve the SDK's own module path to the prepared sources.
	moduleRoot := filepath.Join(distRoot, "vendor", "github.com", "s4wave")
	if err := os.MkdirAll(moduleRoot, 0o755); err != nil {
		return err
	}
	return os.Symlink(distRoot, filepath.Join(moduleRoot, "spacewave"))
}

// retainBuild retains the source DAG with the completed build result and, when
// the submitter asked for it, installs the artifact in the same transaction.
// Execution cleanup must not remove the only local root of an installed plugin's input.
// A failure before the commit leaves the installed plugins unchanged.
func (h *buildPluginHandler) retainBuild(ctx context.Context, key string, ref *manifest.ManifestRef, result *builder.BuilderResult) (*bucket.ObjectRef, error) {
	// Stage the source copy until the transaction that records it commits:
	// a large or peer backed source can take longer than the Volume's grace
	// period to copy.
	stage, err := h.engine.StageWorldState(ctx)
	if err != nil {
		return nil, err
	}
	defer stage.Release()

	// Use the repository result or read the native project's stored provenance.
	tx, err := h.engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	if result == nil {
		result, _, err = resultworld.LookupManifestBuildResult(ctx, tx, key)
		if err != nil {
			return nil, err
		}
	}
	if !result.GetManifestRef().EqualVT(ref) {
		return nil, errors.New("build result does not identify the completed manifest")
	}

	// Localize the source and record it in the build result.
	if err := h.retainSource(ctx, stage, result); err != nil {
		return nil, err
	}
	resultRef, err := resultworld.SetManifestBuildResult(ctx, tx, key, result)
	if err != nil {
		return nil, err
	}

	// Install the immutable artifact under the submitter's authority.
	if h.installer != "" {
		_, err = space_world_ops.InstallSpacePlugin(ctx, tx, h.installer, h.manifestID, key, time.Now())
		if err != nil {
			return nil, errors.Wrap(err, "install plugin build")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return resultRef, nil
}

// validatePluginBuildMode checks the fields of an immutable build or, when the
// config carries a frontend attachment, of a live compiler.
func validatePluginBuildMode(conf *PluginBuildConfig, b bus.Bus) error {
	// An immutable build names a platform and an explicit capacity.
	if conf.GetFrontendId() == "" {
		if err := validateBuildPlatform(conf.GetPlatformId()); err != nil {
			return err
		}
		if conf.GetMilliCpu() == 0 || conf.GetMemoryBytes() == 0 {
			return errors.New("plugin build requires an explicit capacity request")
		}
		return nil
	}

	// A live compiler serves one Session over the device network bus.
	if _, err := uuid.FromString(conf.GetFrontendId()); err != nil {
		return errors.Wrap(err, "invalid frontend attachment ID")
	}
	if _, err := confparse.ParsePeerID(conf.GetFrontendPeerId()); err != nil {
		return errors.Wrap(err, "invalid frontend attachment Session")
	}
	if b == nil {
		return errors.New("live frontend requires the device network bus")
	}
	return nil
}

// validateBuildPlatform accepts the JavaScript platform or a native platform ID.
func validateBuildPlatform(platformID string) error {
	if platformID == bldr_platform.PlatformID_JS {
		return nil
	}
	if _, err := bldr_platform.ParseNativePlatform(platformID); err != nil {
		return errors.Wrap(err, "invalid plugin build platform")
	}
	return nil
}

// retainSource localizes the exact input DAG through stage and makes
// provenance its retention owner once the caller's transaction commits.
func (h *buildPluginHandler) retainSource(ctx context.Context, stage world.WorldStage, result *builder.BuilderResult) error {
	// Reusing a compiled artifact preserves the source that originally produced it.
	if !result.GetSourceRef().GetEmpty() {
		return nil
	}
	return stage.AccessWorldState(ctx, nil, func(dest *bucket_lookup.Cursor) error {
		return h.handle.AccessStorage(ctx, h.source.GetRootRef(), func(source *bucket_lookup.Cursor) error {
			// Copy the source DAG into the World bucket when it lives elsewhere.
			ref := source.GetRef().Clone()
			if source.GetOpArgs().GetBucketId() != dest.GetOpArgs().GetBucketId() {
				var err error
				ref, err = bucket_lookup.CopyObjectToBucket(ctx, dest, source, unixfs_block.NewFSNodeBlock, 4, false, nil)
				if err != nil {
					return err
				}
			}

			// Relative references participate in this bucket's block retention graph.
			ref.BucketId = ""
			result.SourceRef = ref
			return nil
		})
	})
}

// materializeSource reads the immutable input through the execution's storage capability.
// The Execution retains queued input; accepted build provenance retains its local DAG.
func (h *buildPluginHandler) materializeSource(ctx context.Context, directory string) error {
	return h.handle.AccessStorage(ctx, h.source.GetRootRef(), func(cursor *bucket_lookup.Cursor) error {
		// Sync the source tree into the build directory.
		filesystem := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, cursor.Clone(), nil)
		handle, err := unixfs.NewFSHandle(filesystem)
		if err != nil {
			filesystem.Release()
			return err
		}
		defer handle.Release()
		return unixfs_sync.Sync(ctx, directory, handle, unixfs_sync.DeleteMode_DeleteMode_NONE, nil)
	})
}

// _ verifies the plugin build handler contract.
var _ Handler = (*buildPluginHandler)(nil)
