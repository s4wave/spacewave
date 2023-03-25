package bldr_project_controller

import (
	"context"
	"errors"
	"path"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	manifest_builder_controller "github.com/aperturerobotics/bldr/manifest/builder/controller"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
)

// manifestBuilderTracker tracks a running manifest build controller.
type manifestBuilderTracker struct {
	// c is the controller
	c *Controller
	// meta is the manifest meta to build
	meta *bldr_manifest.ManifestMeta
	// resultPromise contains the result of the compilation.
	resultPromise *promise.PromiseContainer[*manifest_builder.BuilderResult]
}

// newManifestBuilderTracker constructs a new build controller tracker.
func (c *Controller) newManifestBuilderTracker(key string) (keyed.Routine, *manifestBuilderTracker) {
	meta, _ := bldr_manifest.UnmarshalManifestMetaB58(key)
	tr := &manifestBuilderTracker{
		c:             c,
		meta:          meta,
		resultPromise: promise.NewPromiseContainer[*manifest_builder.BuilderResult](),
	}
	return tr.execute, tr
}

// execute executes the tracker.
func (t *manifestBuilderTracker) execute(ctx context.Context) error {
	t.resultPromise.SetPromise(nil)

	// build world engine handle
	worldEng := world.NewBusEngine(ctx, t.c.bus, t.c.c.GetEngineId())
	defer worldEng.Close()
	ws := world.NewEngineWorldState(ctx, worldEng, true)

	// set config fields
	meta := t.meta.CloneVT()
	manifestID := meta.GetManifestId()
	if manifestID == "" {
		return bldr_manifest.ErrEmptyManifestID
	}

	pluginWorkingPath := path.Join(t.c.c.GetWorkingPath(), "build", manifestID)
	distSrcPath := path.Join(t.c.c.GetWorkingPath(), "bldr")

	// load plugin config from project config
	manifestConfigs := t.c.c.GetProjectConfig().GetManifests()
	manifestConfig := manifestConfigs[manifestID]

	// determine plugin revision from previous version
	pluginRev := manifestConfig.GetRev()
	pluginPlatformID := meta.GetPlatformId()
	linkObjKeys := t.c.c.GetLinkObjectKeys()
	if len(linkObjKeys) == 0 || linkObjKeys[0] == "" {
		return errors.New("at least one non-empty linked object key is required")
	}

	existingManifests, _, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		ws,
		manifestID,
		pluginPlatformID,
		linkObjKeys...,
	)
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
	meta.Rev = pluginRev
	pluginHostKey := linkObjKeys[0]
	manifestKey := bldr_manifest.NewManifestKey(pluginHostKey, meta)
	builderConf := manifest_builder_controller.NewConfig(
		t.c.c.ToBuilderConfig(
			meta,
			manifestKey,
			distSrcPath,
			pluginWorkingPath,
		),
		manifestConfig.GetBuilder(),
		t.c.c.GetBuildBackoff(),
		t.c.c.GetWatch(),
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

	builderCtrl, ok := ctrlInter.(*manifest_builder_controller.Controller)
	if !ok {
		err := errors.New("unexpected controller type for plugin builder controller")
		t.resultPromise.SetResult(nil, err)
		return err
	}

	resultPromise := builderCtrl.GetResultPromise()
	t.resultPromise.SetPromise(resultPromise)
	/*
		_, err = resultPromise.Await(ctx)
		if err != nil {
			return err
		}
	*/

	// wait for ctx to be canceled
	// this allows the builder controller to resolve FetchPlugin
	<-ctx.Done()
	return nil
}
