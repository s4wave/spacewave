//go:build !js

package devtool

import (
	"context"
	"os"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host_default "github.com/aperturerobotics/bldr/plugin/host/default"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
)

// ExecuteNativeProject starts the project as a native app.
func (a *DevtoolArgs) ExecuteNativeProject(ctx context.Context) error {
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
	b, err := BuildDevtoolBus(ctx, le, stateDir, a.Watch)
	if err != nil {
		return err
	}
	defer b.Release()

	// sync dist sources
	if err := b.SyncDistSources(a.BldrVersion, a.BldrVersionSum, a.BldrSrcPath); err != nil {
		return err
	}

	// write the banner
	writeBanner()

	// start the plugin storage volume
	pluginVolumeID := bldr_plugin.PluginVolumeID
	_, pluginStorageCtrlRef, err := b.StartStorageVolume(ctx, "plugins", &volume_controller.Config{
		VolumeIdAlias: []string{bldr_plugin.PluginVolumeID},
	})
	if err != nil {
		return err
	}
	defer pluginStorageCtrlRef.Release()

	// build the plugin scheduler
	_, relSched, err := plugin_host_default.StartPluginScheduler(
		ctx,
		b.GetBus(),
		b.GetWorldEngineID(),
		b.GetPluginHostObjectKey(),
		pluginVolumeID,
		b.GetVolume().GetPeerID().String(),
		true,
		true,
		true,
	)
	if err != nil {
		return err
	}
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

	// execute the project controller
	// the web plugin will start the appropriate runtime based on BLDR_WEB_RENDERER
	_, projCtrlRef, err := b.StartProjectController(
		ctx,
		b.GetBus(),
		repoRoot,
		a.ConfigPath,
		a.Remote,
	)
	if err != nil {
		return err
	}
	defer projCtrlRef.Release()

	<-b.GetContext().Done()
	return nil
}
