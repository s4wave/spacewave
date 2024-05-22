//go:build !js

package devtool

import (
	"context"

	plugin_host_default "github.com/aperturerobotics/bldr/plugin/host/default"
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

	// build the plugin host controller
	_, relPluginHost, err := plugin_host_default.StartBusPluginHost(
		ctx,
		b.GetBus(),
		b.GetWorldEngineID(),
		b.GetPluginHostObjectKey(),
		b.GetVolume().GetID(),
		b.GetVolume().GetPeerID().String(),
		b.GetPluginsStateRoot(),
		b.GetPluginsDistRoot(),
		true,
		true,
		true,
		"",
	)
	if err != nil {
		return err
	}
	if relPluginHost != nil {
		defer relPluginHost()
	}

	// execute the project controller
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
