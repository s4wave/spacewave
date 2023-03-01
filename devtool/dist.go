package devtool

import (
	"context"
	"os"
	"path"

	dist_compiler "github.com/aperturerobotics/bldr/dist/compiler"
	dist_platform "github.com/aperturerobotics/bldr/dist/platform"
	"github.com/aperturerobotics/bldr/plugin"
	bldr_project_controller "github.com/aperturerobotics/bldr/project/controller"
	"github.com/aperturerobotics/hydra/bucket"
)

// DistProject builds a dist bundle of the project to dist/ with the given platform ID.
func (a *DevtoolArgs) DistProject(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger

	a.Watch = false                                // explicitly disable watching during dist
	a.BuildType = string(plugin.BuildType_RELEASE) // explicitly set release build type
	a.MinifyEntrypoint = true                      // explicitly minify entrypoint during dist

	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the storage + bus
	b, err := BuildDevtoolBus(ctx, le, stateDir, a.Watch)
	if err != nil {
		return err
	}

	if err := b.SyncDistSources(a.BldrVersion, a.BldrVersionSum); err != nil {
		return err
	}
	defer b.Release()

	// read the bldr go.mod
	baseGoMod, err := os.ReadFile(path.Join(b.GetWebSrcDir(), "go.mod"))
	if err != nil {
		return err
	}

	// read the bldr go.sum
	baseGoSum, err := os.ReadFile(path.Join(b.GetWebSrcDir(), "go.sum"))
	if err != nil {
		return err
	}

	// write the banner
	writeBanner()

	// determine the plugin platform ID corresponding to the given dist platform ID
	pluginPlatformID, err := dist_platform.GetPluginPlatformID(a.DistPlatformID)
	if err != nil {
		return err
	}

	// build the working dir
	distRoot := path.Join(stateDir, "dist")
	buildRoot := path.Join(distRoot, "build", a.DistPlatformID)
	outputRoot := path.Join(a.OutputPath, "dist", a.DistPlatformID)

	// create / clean the directories
	if err := os.MkdirAll(buildRoot, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(outputRoot); err == nil {
		if err := os.RemoveAll(outputRoot); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(outputRoot, 0755); err != nil {
		return err
	}

	// execute the project controller
	// compiles the plugins and stores them in the devtool bus world
	projWatcher, projWatcherRef, err := b.StartProjectController(
		ctx,
		b.GetBus(),
		false,
		repoRoot,
		a.ConfigPath,
		pluginPlatformID,
		a.BuildType,
	)
	if err != nil {
		return err
	}
	defer projWatcherRef.Release()

	// get the project controller from the watcher
	projCtrl, err := projWatcher.GetProjectController().WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// get the project config
	projCtrlConf := projCtrl.GetConfig()
	projConf := projCtrlConf.GetProjectConfig()
	appID := projConf.GetId()

	// determine the list of plugins to embed in the entrypoint.
	// default: the list of plugins in the start.plugins list.
	embedPluginsList := projConf.GetEmbedPluginsList()

	// determine the list of plugins to start on startup
	// default: same as the embed plugins list.
	startupPluginsList := embedPluginsList

	// add references to build the embedded plugins
	embedPluginRefs := make([]*bldr_project_controller.PluginBuilderRef, len(embedPluginsList))
	for i, pluginID := range embedPluginsList {
		embedPluginRefs[i] = projCtrl.AddPluginBuilderRef(pluginID)
		defer embedPluginRefs[i].Release() // ensure we release this after
	}

	// wait for the plugins to finish compiling
	le.Infof("waiting for plugins to compile: %v", embedPluginsList)
	embedPluginManifests := make([]*bucket.ObjectRef, len(embedPluginsList))
	for i, pluginBuilderRef := range embedPluginRefs {
		builderCtrlProm := pluginBuilderRef.GetBuilderCtrlPromise()
		builderCtrl, err := builderCtrlProm.Await(ctx)
		if err != nil {
			return err
		}
		resultProm := builderCtrl.GetResultPromise()
		result, err := resultProm.Await(ctx)
		if err != nil {
			return err
		}
		embedPluginManifests[i] = result.PluginManifestRef
	}

	le.Infof("compiled %v plugins to statically embed", len(embedPluginManifests))
	err = dist_compiler.BuildDistBundle(
		ctx,
		le,
		baseGoMod,
		baseGoSum,
		buildRoot,
		outputRoot,
		b.GetWorldState(),
		a.DistPlatformID,
		embedPluginManifests,
		startupPluginsList,
		appID,
	)
	if err != nil {
		return err
	}

	// cleanup: remove working path
	_ = os.RemoveAll(buildRoot)
	return nil
}
