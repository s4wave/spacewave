//go:build !js

package devtool

import (
	"context"
	"os"
	"slices"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_compiler_js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
)

// ExecuteNativeProject starts the project as a native app.
func (a *DevtoolArgs) ExecuteNativeProject(ctx context.Context) (err error) {
	// init repo root and storage directories
	le := a.Logger
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("starting with state dir: %s", stateDir)

	// Set web renderer env var if specified (non-empty).
	// This is read by the web plugin compiler to decide which runtime to bundle.
	if a.WebRenderer != "" {
		renderer, err := web_runtime.ParseWebRenderer(a.WebRenderer)
		if err != nil {
			return err
		}
		resolved := renderer.Resolve()
		le.Infof("using web renderer: %s", resolved.String())
		if err := os.Setenv(web_runtime.WebRendererEnvVar, resolved.String()); err != nil {
			return err
		}
	}

	// initialize the storage + bus
	b, err := BuildDevtoolBus(ctx, le, repoRoot, stateDir, a.Watch)
	if err != nil {
		return err
	}
	defer b.Release()
	commandLogFile := a.commandLogFile()
	b.setCommandStartingWithLogFile("start desktop", "initializing desktop runtime", commandLogFile)
	ctx, stopTUI := a.startDevtoolTUI(ctx, b.GetStatusProducer(), "")
	defer func() {
		b.finishCommandWithLogFile(ctx, "start desktop", commandLogFile, err)
		stopTUI()
	}()

	// sync dist sources
	err = b.SyncDistSources(a.BldrVersion, a.BldrVersionSum, a.BldrSrcPath)
	if err != nil {
		return err
	}

	// write the banner
	a.writeBannerTo(os.Stderr)

	// start the plugin storage volume
	pluginVolumeID := bldr_plugin.PluginVolumeID
	_, pluginStorageCtrlRef, err := b.StartStorageVolume(ctx, "plugins", &volume_controller.Config{
		VolumeIdAlias: []string{bldr_plugin.PluginVolumeID},
	})
	if err != nil {
		return err
	}
	defer pluginStorageCtrlRef.Release()

	// execute the project controller
	// the web plugin will start the appropriate runtime based on BLDR_WEB_RENDERER
	projWatcher, projCtrlRef, err := b.StartProjectControllerWithStartup(
		ctx,
		b.GetBus(),
		repoRoot,
		a.ConfigPath,
		a.Remote,
		a.StartPlugins.Value(),
		false,
	)
	if err != nil {
		return err
	}
	defer projCtrlRef.Release()

	projCtrl, err := projWatcher.GetProjectController().WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	preflightRemote := a.Remote
	if preflightRemote == "" {
		preflightRemote = "devtool"
	}
	startPlugins := projectOwnedStartupPlugins(projCtrl.GetConfig().GetProjectConfig())
	if len(startPlugins) != 0 {
		le.WithField("plugin-count", len(startPlugins)).Info("preflighting startup manifests")
		_, _, err = projCtrl.BuildManifests(
			ctx,
			preflightRemote,
			startPlugins,
			bldr_manifest.BuildType(a.BuildType),
			nil,
		)
		if err != nil {
			return err
		}
	}

	// build the plugin scheduler
	sched, relSched, err := plugin_host_default.StartNativeDesktopPluginScheduler(
		ctx,
		b.GetBus(),
		"",
		b.GetWorldEngineID(),
		b.GetPluginHostObjectKey(),
		pluginVolumeID,
		b.GetVolume().GetPeerID().String(),
		true,
		true,
		true,
		nativeDesktopQuickJSPluginIDs(projCtrl.GetConfig().GetProjectConfig()),
	)
	if err != nil {
		return err
	}
	devtool_status.AttachPluginStatus(ctx, b.GetStatusProducer(), sched)
	defer relSched()

	// build the plugin host controller
	_, relPluginHost, err := plugin_host_default.StartPluginHost(
		ctx,
		b.GetBus(),
		b.GetPluginsStateRoot(),
		b.GetPluginsDistRoot(),
		"",
	)
	if err != nil {
		return err
	}
	if relPluginHost != nil {
		defer relPluginHost()
	}

	projCtrl.StartStartup(ctx)
	b.setCommandRunningWithLogFile("start desktop", "desktop runtime active", commandLogFile)

	<-b.GetContext().Done()
	return nil
}

func nativeDesktopQuickJSPluginIDs(projectConfig *bldr_project.ProjectConfig) []string {
	pluginIDs := make([]string, 0)
	for pluginID, manifest := range projectConfig.GetManifests() {
		if manifest.GetBuilder().GetId() != bldr_plugin_compiler_js.ConfigID {
			continue
		}
		pluginIDs = append(pluginIDs, pluginID)
	}
	slices.Sort(pluginIDs)
	return pluginIDs
}
