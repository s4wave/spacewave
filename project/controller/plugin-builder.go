package bldr_project_controller

import (
	"context"
	"errors"
	"path"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_builder "github.com/aperturerobotics/bldr/plugin/builder"
	plugin_builder_controller "github.com/aperturerobotics/bldr/plugin/builder/controller"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
)

// pluginBuilderTracker tracks a running plugin build controller.
type pluginBuilderTracker struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
	// resultPromise contains the result of the compilation.
	resultPromise *promise.PromiseContainer[*plugin_builder.PluginBuilderResult]
}

// newPluginBuilderTracker constructs a new plugin build controller tracker.
func (c *Controller) newPluginBuilderTracker(key string) (keyed.Routine, *pluginBuilderTracker) {
	tr := &pluginBuilderTracker{
		c:             c,
		pluginID:      key,
		resultPromise: promise.NewPromiseContainer[*plugin_builder.PluginBuilderResult](),
	}
	return tr.execute, tr
}

// execute executes the tracker.
func (t *pluginBuilderTracker) execute(ctx context.Context) error {
	t.resultPromise.SetPromise(nil)

	// build world engine handle
	worldEng := world.NewBusEngine(ctx, t.c.bus, t.c.c.GetEngineId())
	defer worldEng.Close()
	ws := world.NewEngineWorldState(ctx, worldEng, true)

	// set config fields
	pluginID := t.pluginID
	pluginWorkingPath := path.Join(t.c.c.GetWorkingPath(), "plugin", "build", pluginID)
	distSrcPath := path.Join(t.c.c.GetWorkingPath(), "bldr")

	// load plugin config from project config
	pluginConfigs := t.c.c.GetProjectConfig().GetPlugins()
	pluginConfig := pluginConfigs[pluginID]

	// determine plugin revision from previous version
	pluginRev := pluginConfig.GetRev()
	pluginPlatformID := t.c.c.GetPluginPlatformId()
	existingManifests, _, err := plugin_host.CollectPluginManifestsForPluginID(ctx, ws, pluginID, pluginPlatformID, t.c.c.GetPluginHostKey())
	if err != nil {
		return err
	}
	if len(existingManifests) != 0 {
		existingManifest := existingManifests[0]
		if existingRev := existingManifest.GetRev(); existingRev >= pluginRev {
			pluginRev = existingRev + 1
		}
	}

	// build plugin manifest metadata and builder config
	meta := t.c.c.ToPluginManifestMeta(pluginID, pluginRev)
	manifestKey := bldr_plugin.NewPluginManifestKey(t.c.c.GetPluginHostKey(), meta)
	builderConf := plugin_builder_controller.NewConfig(
		t.c.c.ToPluginBuilderConfig(
			meta,
			manifestKey,
			distSrcPath,
			pluginWorkingPath,
		),
		pluginConfig.GetBuilder(),
		t.c.c.GetBuildBackoff(),
	)

	ctrlInter, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		t.c.bus,
		resolver.NewLoadControllerWithConfig(builderConf),
		nil,
	)
	if err != nil {
		t.resultPromise.SetResult(nil, err)
		return err
	}
	defer ctrlRef.Release()

	builderCtrl, ok := ctrlInter.(*plugin_builder_controller.Controller)
	if !ok {
		err := errors.New("unexpected controller type for plugin builder controller")
		t.resultPromise.SetResult(nil, err)
		return err
	}

	resultPromise := builderCtrl.GetResultPromise()
	t.resultPromise.SetPromise(resultPromise)
	_, err = resultPromise.Await(ctx)
	if err != nil {
		return err
	}

	// wait for ctx to be canceled
	// this allows the builder controller to resolve FetchPlugin
	<-ctx.Done()
	return nil
}
