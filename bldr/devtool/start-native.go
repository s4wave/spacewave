//go:build !js

package devtool

import (
	"context"
	"os"
	"slices"

	"github.com/pkg/errors"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_admit "github.com/s4wave/spacewave/bldr/plugin/admit"
	bldr_plugin_compiler_js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/sirupsen/logrus"
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
	b, err := a.buildDevtoolBus(ctx, le, repoRoot, stateDir, a.Watch)
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

	// Wait for the evaluated project configuration.
	projCtrl, err := projWatcher.WaitProjectController(ctx)
	if err != nil {
		return err
	}

	// Preflight the project-owned startup plugin manifests before the scheduler
	// starts, so startup keeps validated cached manifests.
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

	// Bind the scheduler to the devtool World and the project's startup plugins.
	schedConf := b.newNativeSchedulerConfig(projCtrl.GetConfig().GetProjectConfig(), pluginVolumeID)

	// Startup plugins may run their own plugins, such as a Space's, on the
	// devtool's plugin hosts.
	schedConf.HostExportPluginIds = startPlugins
	schedConf.AdmissionPluginId = projCtrl.GetConfig().GetProjectConfig().GetStart().GetAdmissionPluginId()

	// Ask the project's approvals owner about every root the scheduler loads.
	if owner := projCtrl.GetConfig().GetProjectConfig().GetStart().GetAdmissionPluginId(); owner != "" {
		relAdmit, err := b.mountPluginAdmission(ctx, le, owner)
		if err != nil {
			return err
		}
		defer relAdmit()
	}

	// Start the plugin scheduler and attach its status to the devtool producer.
	relSched, err := b.startNativePluginScheduler(ctx, schedConf)
	if err != nil {
		return err
	}
	defer relSched()

	// Start the native plugin host with the devtool's state and dist roots.
	relPluginHost, err := b.startNativePluginHost(ctx)
	if err != nil {
		return err
	}
	if relPluginHost != nil {
		defer relPluginHost()
	}

	// Start the project, open the desktop window, and publish the running status.
	projCtrl.StartStartup(ctx)
	desktopErr := make(chan error, 1)
	go func() {
		desktopErr <- b.openDesktopWindow(ctx)
	}()
	b.setCommandRunningWithLogFile("start desktop", "desktop runtime active", commandLogFile)

	// Block until the devtool bus context ends or the desktop window fails to
	// open, since nothing else launches Electron.
	return b.waitForDesktop(desktopErr)
}

// newNativeSchedulerConfig binds the desktop scheduler to the devtool World.
func (d *DevtoolBus) newNativeSchedulerConfig(project *bldr_project.ProjectConfig, pluginVolumeID string) *plugin_host_scheduler.Config {
	return plugin_host_default.NewNativeDesktopSchedulerConfig(
		"", d.worldEngineID, d.pluginHostObjectKey, pluginVolumeID,
		d.peerID.String(), true, true, true, nativeDesktopQuickJSPluginIDs(project),
	)
}

// mountPluginAdmission lets the project's approval plugin admit loaded roots.
func (d *DevtoolBus) mountPluginAdmission(ctx context.Context, le *logrus.Entry, owner string) (func(), error) {
	return d.AddController(ctx, bldr_plugin_admit.NewController(le, d.b, owner))
}

// startNativePluginScheduler starts the scheduler and attaches its status.
func (d *DevtoolBus) startNativePluginScheduler(ctx context.Context, conf *plugin_host_scheduler.Config) (func(), error) {
	// Start the scheduler before subscribing to its status.
	sched, release, err := plugin_host_default.StartPluginSchedulerWithConfig(ctx, d.b, conf)
	if err != nil {
		return nil, err
	}

	// Publish scheduler status through the devtool status producer.
	devtool_status.AttachPluginStatus(ctx, d.statusProducer, sched)
	return release, nil
}

// startNativePluginHost starts the host with the devtool's state and dist roots.
func (d *DevtoolBus) startNativePluginHost(ctx context.Context) (func(), error) {
	_, release, err := plugin_host_default.StartPluginHost(ctx, d.b, d.pluginsStateRoot, d.pluginsDistRoot, "")
	return release, err
}

// waitForDesktop returns when the bus stops or opening the window fails.
func (d *DevtoolBus) waitForDesktop(desktopErr <-chan error) error {
	select {
	case <-d.ctx.Done():
		return nil
	case err := <-desktopErr:
		return err
	}
}

// openDesktopWindow asks the web plugin to open the main window once it loads.
// Electron starts only on demand, so the devtool supplies the demand and holds
// the web plugin reference until ctx ends. It returns nil when ctx ends and an
// error when the web plugin cannot load or open the window.
func (d *DevtoolBus) openDesktopWindow(ctx context.Context) error {
	// Wait for the web plugin to load and hold its client reference.
	client, ref, err := bldr_plugin.ExPluginLoadWaitClient(ctx, d.b, "web", nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return errors.Wrap(err, "load web plugin to open desktop window")
	}
	defer ref.Release()

	// Ask the web plugin to open or focus the desktop window.
	_, err = bldr_web_plugin.NewSRPCWebPluginClient(client).
		OpenOrFocusDesktop(ctx, &bldr_web_plugin.OpenOrFocusDesktopRequest{})
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return errors.Wrap(err, "open desktop window")
	}

	// Hold the reference until the context ends.
	<-ctx.Done()
	return nil
}

// nativeDesktopQuickJSPluginIDs returns the QuickJS plugin ids needed by a
// native desktop build.
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
