//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

type subManifestBuildOwner struct {
	tracker        *subManifestBuilderTracker
	builderRoutine *routine.StateRoutineContainer[*bldr_project.ManifestConfig]
	resultPc       *promise.PromiseContainer[*bldr_manifest_builder.BuilderResult]

	mtx       sync.Mutex
	restartFn func(string)
	result    *bldr_manifest_builder.BuilderResult
	resultErr error
	observed  bool
}

func newSubManifestBuildOwner(tracker *subManifestBuilderTracker) *subManifestBuildOwner {
	owner := &subManifestBuildOwner{
		tracker:  tracker,
		resultPc: promise.NewPromiseContainer[*bldr_manifest_builder.BuilderResult](),
	}
	owner.builderRoutine = routine.NewStateRoutineContainerWithLoggerVT[*bldr_project.ManifestConfig](tracker.c.le)
	owner.builderRoutine.SetStateRoutine(tracker.executeBuilderRoutine)
	return owner
}

func (o *subManifestBuildOwner) setContext(ctx context.Context) {
	o.builderRoutine.SetContext(ctx, true)
}

func (o *subManifestBuildOwner) setManifestConfig(
	manifestConf *bldr_project.ManifestConfig,
	restartFn func(string),
) (*promise.PromiseContainer[*bldr_manifest_builder.BuilderResult], error) {
	o.mtx.Lock()
	defer o.mtx.Unlock()

	_, changed, _, _ := o.builderRoutine.SetState(manifestConf)
	if changed && o.observed && (o.result != nil || o.resultErr != nil) {
		// don't allow this, could cause infinite loops
		return nil, errors.New("called BuildSubManifest with different configuration after a value was already resolved")
	}

	o.observed = true
	if restartFn != nil {
		o.restartFn = restartFn
	}

	return o.resultPc, nil
}

func (o *subManifestBuildOwner) prepareParentAttempt(restartFn func(string)) {
	o.mtx.Lock()
	o.restartFn = restartFn
	o.observed = false
	o.mtx.Unlock()
}

func (o *subManifestBuildOwner) observedInParentAttempt() bool {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	return o.observed
}

func (o *subManifestBuildOwner) setResult(val *bldr_manifest_builder.BuilderResult, err error) {
	o.mtx.Lock()
	defer o.mtx.Unlock()

	if o.result.EqualVT(val) && o.resultErr == err {
		return
	}

	if o.observed && (o.result != nil || o.resultErr != nil) {
		if o.restartFn != nil {
			o.restartFn("sub-manifest changed: " + o.tracker.subManifestID)
			o.restartFn = nil
		}
		o.observed = false
	}

	o.result = val
	o.resultErr = err
	o.resultPc.SetResult(val, err)
}
