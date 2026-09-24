//go:build !js && !tinygo

package space_exec

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"

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
	project "github.com/s4wave/spacewave/bldr/project"
	project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/util/confparse"
	app_web "github.com/s4wave/spacewave/web"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

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
	// configPath identifies the project configuration inside source.
	configPath string
	// bus supplies the assigned device's authenticated transport.
	bus bus.Bus
	// config selects an immutable build or a retained frontend attachment.
	config *PluginBuildConfig
}

// NewBuildPluginHandlerWithBus binds the build to the execution's World capability.
// The bus supplies the assigned device's network transport for live authoring.
func NewBuildPluginHandlerWithBus(_ context.Context, b bus.Bus, le *logrus.Entry, _ world.WorldState, handle forge_target.ExecControllerHandle, inputs forge_target.InputMap, data []byte) (Handler, error) {
	conf := &PluginBuildConfig{}
	if err := conf.UnmarshalJSON(data); err != nil {
		return nil, errors.Wrap(err, "parse plugin build config")
	}
	inputSource, err := forge_target.InputValueToValue(inputs["source"])
	if err != nil {
		return nil, err
	}
	source := inputSource.GetWorldObjectSnapshot()
	if source.GetRootRef() == nil || source.GetObjectType() != unixfs_world.FSNodeTypeID {
		return nil, errors.New("plugin source must be a pinned Space UnixFS directory")
	}
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
	if conf.GetFrontendId() != "" {
		if _, err := uuid.FromString(conf.GetFrontendId()); err != nil {
			return nil, errors.Wrap(err, "invalid frontend attachment ID")
		}
		if _, err := confparse.ParsePeerID(conf.GetFrontendPeerId()); err != nil {
			return nil, errors.Wrap(err, "invalid frontend attachment Session")
		}
		if b == nil {
			return nil, errors.New("live frontend requires the device network bus")
		}
	}
	input, ok := inputs["world"].(forge_target.InputValueWorld)
	if !ok || input.GetWorldEngine() == nil {
		return nil, errors.New("plugin build requires a transactional World input")
	}
	return &buildPluginHandler{
		le: le, engine: input.GetWorldEngine(), handle: handle,
		source: source.CloneVT(), manifestID: manifestID, configPath: configPath,
		bus: b, config: conf,
	}, nil
}

// Execute retains exact source, then lets Bldr write artifacts into the supplied World.
// The temporary host checkout is disposable; accepted artifacts never depend on it.
func (h *buildPluginHandler) Execute(ctx context.Context) error {
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
	sourceRoot := filepath.Join(working, "source")
	stateRoot := filepath.Join(sourceRoot, ".bldr")
	buildKey := path.Join(h.source.GetKey(), "builds")
	source, err := h.materializeSource(ctx, sourceRoot)
	if err != nil {
		return errors.Wrap(err, "materialize plugin source")
	}
	if err := h.handle.WriteLog(ctx, "info", "Building "+h.manifestID+" from "+source.GetKey()); err != nil {
		return err
	}

	// Prepare the shipped TypeScript SDK without requiring a Go checkout in the Space.
	buildBus, err := devtool.BuildDevtoolBus(ctx, h.le, sourceRoot, stateRoot, live)
	if err != nil {
		return err
	}
	defer buildBus.Release()
	distRoot := filepath.Join(stateRoot, "src")
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
	moduleRoot := filepath.Join(distRoot, "vendor", "github.com", "s4wave")
	if err := os.MkdirAll(moduleRoot, 0o755); err != nil {
		return err
	}
	if err := os.Symlink(distRoot, filepath.Join(moduleRoot, "spacewave")); err != nil {
		return err
	}

	// Compile directly into the accepted World, preserving child build results and DAG refs.
	const engineID = "plugin-build-space"
	releaseEngine, err := buildBus.GetBus().AddController(ctx, world.NewEngineController(engineID, h.engine), nil)
	if err != nil {
		return err
	}
	defer releaseEngine()
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
	if live {
		return h.runFrontend(ctx, sourceRoot, compiler.GetFrontendService())
	}
	refs, keys, err := compiler.BuildManifestBuilderConfigs(ctx, []*project_controller.ManifestBuilderConfig{
		project_controller.NewManifestBuilderConfig(h.manifestID, string(manifest.BuildType_DEV), "js", "space"),
	})
	if err != nil {
		return err
	}
	if len(refs) != 1 || len(keys) != 1 {
		return errors.New("plugin compiler did not produce one JavaScript manifest")
	}

	// Retain source with the artifact before reporting success. Execution cleanup
	// must not remove the only local root for an installed plugin's exact input.
	tx, err := h.engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	result, _, err := resultworld.LookupManifestBuildResult(ctx, tx, keys[0])
	if err != nil {
		return err
	}
	if !result.GetManifestRef().EqualVT(refs[0]) {
		return errors.New("build result does not identify the completed manifest")
	}
	if err := h.retainSource(ctx, tx, result); err != nil {
		return err
	}
	resultRef, err := resultworld.SetManifestBuildResult(ctx, tx, keys[0], result)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return h.handle.SetOutputs(ctx, forge_value.ValueSlice{
		forge_value.NewValueWithWorldObjectSnapshot("source", source),
		forge_value.NewValueWithWorldObjectSnapshot("manifest", &forge_value.WorldObjectSnapshot{
			Key: keys[0], RootRef: refs[0].GetManifestRef(), ObjectType: "bldr/manifest",
		}),
		forge_value.NewValueWithBucketRef("build-result", resultRef),
	}, true)
}

// retainSource localizes the exact input DAG and makes provenance its retention owner.
func (h *buildPluginHandler) retainSource(ctx context.Context, ws world.WorldState, result *builder.BuilderResult) error {
	// Reusing a compiled artifact preserves the source that originally produced it.
	if !result.GetSourceRef().GetEmpty() {
		return nil
	}
	return ws.AccessWorldState(ctx, nil, func(dest *bucket_lookup.Cursor) error {
		return h.handle.AccessStorage(ctx, h.source.GetRootRef(), func(source *bucket_lookup.Cursor) error {
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
func (h *buildPluginHandler) materializeSource(ctx context.Context, directory string) (*forge_value.WorldObjectSnapshot, error) {
	err := h.handle.AccessStorage(ctx, h.source.GetRootRef(), func(cursor *bucket_lookup.Cursor) error {
		filesystem := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, cursor.Clone(), nil)
		handle, err := unixfs.NewFSHandle(filesystem)
		if err != nil {
			filesystem.Release()
			return err
		}
		defer handle.Release()
		return unixfs_sync.Sync(ctx, directory, handle, unixfs_sync.DeleteMode_DeleteMode_NONE, nil)
	})
	if err != nil {
		return nil, err
	}
	return h.source.CloneVT(), nil
}
