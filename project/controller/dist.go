package bldr_project_controller

import (
	"context"
	"os"
	"path"

	dist_compiler "github.com/aperturerobotics/bldr/dist/compiler"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
)

// DistTargets compiles the given dist target(s)
func (c *Controller) DistTargets(
	ctx context.Context,
	distPlatformID,
	target string,
	buildRoot,
	outputRoot string,
) error {
	// TODO

	// determine the list of plugins to embed in the entrypoint.
	// default: the list of plugins in the start.plugins list.
	// embedPluginsList := projConf.GetEmbedPluginsList()
	// TODO
	// determine the list of plugins to start on startup
	// TODO
	projConf := c.c.GetProjectConfig()
	appID := projConf.GetId()
	startupPluginsList := projConf.GetStart().GetPlugins()
	embedPluginsList := startupPluginsList

	// read the bldr go.mod
	distSrcDir := path.Join(c.c.GetWorkingPath(), "bldr")
	baseGoMod, err := os.ReadFile(path.Join(distSrcDir, "go.mod"))
	if err != nil {
		return err
	}

	// read the bldr go.sum
	baseGoSum, err := os.ReadFile(path.Join(distSrcDir, "go.sum"))
	if err != nil {
		return err
	}

	// add references to build the embedded plugins
	embedPluginRefs := make([]*PluginBuilderRef, len(embedPluginsList))
	for i, pluginID := range embedPluginsList {
		embedPluginRefs[i] = c.AddPluginBuilderRef(pluginID)
		defer embedPluginRefs[i].Release() // ensure we release this after
	}

	// wait for the plugins to finish compiling
	c.le.Infof("waiting for plugins to compile: %v", embedPluginsList)
	embedPluginManifests := make([]*bucket.ObjectRef, len(embedPluginsList))
	for i, pluginBuilderRef := range embedPluginRefs {
		resultProm := pluginBuilderRef.GetResultPromise()
		result, err := resultProm.Await(ctx)
		if err != nil {
			return err
		}
		embedPluginManifests[i] = result.PluginManifestRef
	}

	c.le.Infof("compiled %v plugins to statically embed", len(embedPluginManifests))
	worldEngine := c.BuildWorldEngine(ctx)
	defer worldEngine.Close()
	worldState := world.NewEngineWorldState(ctx, worldEngine, true)
	return dist_compiler.BuildDistBundle(
		ctx,
		c.le,
		baseGoMod,
		baseGoSum,
		buildRoot,
		outputRoot,
		worldState,
		distPlatformID,
		embedPluginManifests,
		startupPluginsList,
		appID,
	)
}
