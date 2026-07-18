//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

// subManifestBuilderTracker tracks a running sub-manifest build controller.
type subManifestBuilderTracker struct {
	// c is the controller
	c *Controller
	// subManifestID is the sub-manifest ID
	subManifestID string
	// build owns the child build routine and observed result state.
	build *subManifestBuildOwner
}

// newSubManifestBuilderTracker constructs a new sub-manifest build controller tracker.
func (c *Controller) newSubManifestBuilderTracker(subManifestID string) (keyed.Routine, *subManifestBuilderTracker) {
	tr := &subManifestBuilderTracker{
		c:             c,
		subManifestID: subManifestID,
	}
	tr.build = newSubManifestBuildOwner(tr)
	return tr.execute, tr
}

// execute executes the tracker.
func (t *subManifestBuilderTracker) execute(ctx context.Context) error {
	t.build.setContext(ctx)
	<-ctx.Done() // necessary because Keyed cancels ctx after we return.
	return nil
}

// setManifestConfig updates the manifest config and clears the result if needed
// returns an error if ManifestConfig != current, current was set, and a result was already returned
func (t *subManifestBuilderTracker) setManifestConfig(manifestConf *bldr_project.ManifestConfig, restartFn func(string)) (*promise.PromiseContainer[*bldr_manifest_builder.BuilderResult], error) {
	return t.build.setManifestConfig(manifestConf, restartFn)
}

// executeBuilderRoutine executes the builder directive with the config.
// ctx is canceled if the config changes
func (t *subManifestBuilderTracker) executeBuilderRoutine(ctx context.Context, manifestConfig *bldr_project.ManifestConfig) error {
	// build a combined manifest id for the sub-manifest
	subManifestID := t.subManifestID
	ctrlConf := t.c.GetConfig()
	parentBuilderConfig := ctrlConf.GetBuilderConfig()
	manifestID := strings.Join([]string{parentBuilderConfig.GetManifestMeta().GetManifestId(), subManifestID}, "-")
	if err := bldr_manifest.ValidateManifestID(manifestID, false); err != nil {
		return errors.Wrap(err, "invalid combined sub-manifest id")
	}

	// build plugin manifest metadata and builder config
	meta := parentBuilderConfig.GetManifestMeta().CloneVT()
	meta.ManifestId = manifestID

	// create working path
	workingPath := filepath.Join(parentBuilderConfig.GetWorkingPath(), "sub", subManifestID)
	if err := os.MkdirAll(workingPath, 0o755); err != nil {
		return err
	}

	manifestKey := bldr_manifest.NewSubManifestKey(parentBuilderConfig.GetObjectKey(), subManifestID)
	manifestBuilderConf := parentBuilderConfig.CloneVT()
	manifestBuilderConf.ManifestMeta = meta
	manifestBuilderConf.ObjectKey = manifestKey
	manifestBuilderConf.LinkObjectKeys = nil // TODO should we link this?
	manifestBuilderConf.WorkingPath = workingPath

	var startupBuilderResult *bldr_manifest_builder.BuilderResult
	if parentStartupResult := ctrlConf.GetStartupBuilderResult(); parentStartupResult != nil {
		startupBuilderResult = parentStartupResult.GetSubManifestResults()[subManifestID]
	}

	builderConf := NewConfig(
		manifestBuilderConf,
		manifestConfig.GetBuilder(),
		ctrlConf.GetBuildBackoff(),
		ctrlConf.GetWatch(),
		startupBuilderResult,
	)

	builderCtrl, _, ctrlRef, err := loader.WaitExecControllerRunningTyped[*Controller](
		ctx,
		t.c.bus,
		resolver.NewLoadControllerWithConfig(builderConf),
		nil,
	)
	if ctrlRef != nil {
		defer ctrlRef.Release()
	}
	if ctx.Err() != nil {
		return context.Canceled
	}
	if err != nil {
		t.build.setResult(nil, err)
		return err
	}

	for {
		resultPromiseCtr := builderCtrl.GetResultPromise()
		resultPromise, resultPromiseChanged := resultPromiseCtr.GetPromise()

		if resultPromise != nil {
			result, err := resultPromise.Await(ctx)
			if ctx.Err() != nil {
				return context.Canceled
			}
			if err != nil {
				t.build.setResult(nil, err)
			} else {
				t.build.setResult(result, nil)
			}
			if err != nil {
				return err
			}
		}

		select {
		case <-ctx.Done():
			return context.Canceled
		case <-resultPromiseChanged:
			// re-check (manifest was rebuilt)
		}
	}
}
