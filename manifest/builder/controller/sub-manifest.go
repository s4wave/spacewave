//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
)

// subManifestBuilderTracker tracks a running sub-manifest build controller.
type subManifestBuilderTracker struct {
	// c is the controller
	c *Controller
	// subManifestID is the sub-manifest ID
	subManifestID string

	// builderRoutine contains the manifest builder routine
	builderRoutine *routine.StateRoutineContainer[*bldr_project.ManifestConfig]

	// mtx guards below fields
	mtx sync.Mutex
	// restartFn restarts the active BuildManifest call, if any
	// call after discarding the current result IF the result was returned already
	// makes sure we re-run BuildManifest if any sub-manifests changed after we returned a value
	// may be nil
	restartFn func()
	// resultPc is the result promise container that is returned from BuildSubManifest
	// this pointer does not change
	resultPc *promise.PromiseContainer[*manifest_builder.BuilderResult]
	// result is the current result
	result *manifest_builder.BuilderResult
	// resultErr is the current result error
	resultErr error
	// resultPcObserved indicates we set a result into resultPc already and returned resultPc to a the current BuildManifest iteration
	// call restartFn if changing the result and resultPcObserved was already != nil
	resultPcObserved bool
}

// newSubManifestBuilderTracker constructs a new sub-manifest build controller tracker.
func (c *Controller) newSubManifestBuilderTracker(subManifestID string) (keyed.Routine, *subManifestBuilderTracker) {
	tr := &subManifestBuilderTracker{
		c:             c,
		subManifestID: subManifestID,
	}
	tr.builderRoutine = routine.NewStateRoutineContainerWithLoggerVT[*bldr_project.ManifestConfig](c.le)
	tr.builderRoutine.SetStateRoutine(tr.executeBuilderRoutine)
	tr.resultPc = promise.NewPromiseContainer[*manifest_builder.BuilderResult]()
	return tr.execute, tr
}

// execute executes the tracker.
func (t *subManifestBuilderTracker) execute(ctx context.Context) error {
	t.builderRoutine.SetContext(ctx, true)
	<-ctx.Done() // necessary because Keyed cancels ctx after we return.
	return nil
}

// setManifestConfig updates the manifest config and clears the result if needed
// returns an error if ManifestConfig != current, current was set, and a result was already returned
func (t *subManifestBuilderTracker) setManifestConfig(manifestConf *bldr_project.ManifestConfig, restartFn func()) (*promise.PromiseContainer[*manifest_builder.BuilderResult], error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	_, changed, _, _ := t.builderRoutine.SetState(manifestConf)
	if changed && t.resultPcObserved && (t.result != nil || t.resultErr != nil) {
		// don't allow this, could cause infinite loops
		return nil, errors.New("called BuildSubManifest with different configuration after a value was already resolved")
	}

	// mark the tracker pc as observed
	t.resultPcObserved = true
	if restartFn != nil {
		t.restartFn = restartFn
	}

	return t.resultPc, nil
}

// setResultLocked updates the result and calls restartFn if needed while mtx is locked
func (t *subManifestBuilderTracker) setResultLocked(val *manifest_builder.BuilderResult, err error) {
	// if the result is identical do nothing
	if t.result.EqualVT(val) && t.resultErr == err {
		return
	}

	// check if the result was already set & returned
	if t.resultPcObserved && (t.result != nil || t.resultErr != nil) {
		if t.restartFn != nil {
			t.restartFn()
			t.restartFn = nil
		}
		t.resultPcObserved = false
	}

	// update the result
	t.result = val
	t.resultErr = err
	t.resultPc.SetResult(val, err)
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

	builderConf := NewConfig(
		manifestBuilderConf,
		manifestConfig.GetBuilder(),
		ctrlConf.GetBuildBackoff(),
		ctrlConf.GetWatch(),
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
		t.mtx.Lock()
		t.setResultLocked(nil, err)
		t.mtx.Unlock()
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
			t.mtx.Lock()
			if err != nil {
				t.setResultLocked(nil, err)
			} else {
				t.setResultLocked(result, nil)
			}
			t.mtx.Unlock()
			if err != nil {
				return err
			}
		} else {
			// No result yet.
		}

		select {
		case <-ctx.Done():
			return context.Canceled
		case <-resultPromiseChanged:
			// re-check (manifest was rebuilt)
		}
	}
}
