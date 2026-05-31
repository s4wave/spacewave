//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/promise"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/sirupsen/logrus"
)

type manifestBuildOwner struct {
	c             *Controller
	builderConfig *bldr_manifest_builder.BuilderConfig
	resultPromise *promise.PromiseContainer[*bldr_manifest_builder.BuilderResult]

	prevResult   *bldr_manifest_builder.BuilderResult
	prevErr      error
	changedFiles []*bldr_manifest_builder.InputManifest_File

	mtx           sync.Mutex
	rebuildReason string
	current       *manifestBuildAttempt
}

type manifestBuildAttempt struct {
	owner     *manifestBuildOwner
	ctx       context.Context
	cancel    context.CancelFunc
	restarted bool
}

func newManifestBuildOwner(
	c *Controller,
	builderConfig *bldr_manifest_builder.BuilderConfig,
) *manifestBuildOwner {
	return &manifestBuildOwner{
		c:             c,
		builderConfig: builderConfig,
		resultPromise: c.resultPromise,
	}
}

func (o *manifestBuildOwner) beginAttempt(ctx context.Context) *manifestBuildAttempt {
	attemptCtx, cancel := context.WithCancel(ctx)
	attempt := &manifestBuildAttempt{
		owner:  o,
		ctx:    attemptCtx,
		cancel: cancel,
	}

	o.mtx.Lock()
	o.current = attempt
	o.mtx.Unlock()

	return attempt
}

func (o *manifestBuildOwner) nextResultPromise() *promise.Promise[*bldr_manifest_builder.BuilderResult] {
	resultPromise := promise.NewPromise[*bldr_manifest_builder.BuilderResult]()
	o.resultPromise.SetPromise(resultPromise)
	return resultPromise
}

func (o *manifestBuildOwner) buildArgs() *bldr_manifest_builder.BuildManifestArgs {
	args := &bldr_manifest_builder.BuildManifestArgs{
		BuilderConfig:     o.builderConfig,
		PrevBuilderResult: o.prevResult,
		ChangedFiles:      o.changedFiles,
	}
	o.changedFiles = nil
	return args
}

func (o *manifestBuildOwner) rebuildFlags(result *bldr_manifest_builder.BuilderResult) (bool, bool) {
	if result != nil {
		return false, false
	}
	return o.prevResult == nil, o.prevResult != nil
}

func (o *manifestBuildOwner) setChangedFiles(changedFiles []*bldr_manifest_builder.InputManifest_File) {
	o.changedFiles = changedFiles
	o.setRebuildReason(changedFilesSummary(len(changedFiles)))
}

func (o *manifestBuildOwner) rebuildReasonSnapshot() string {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	return o.rebuildReason
}

func (o *manifestBuildOwner) setRebuildReason(reason string) {
	o.mtx.Lock()
	o.rebuildReason = reason
	o.mtx.Unlock()
}

func (o *manifestBuildOwner) publishResult(
	ctx context.Context,
	le *logrus.Entry,
	resultPromise *promise.Promise[*bldr_manifest_builder.BuilderResult],
	result *bldr_manifest_builder.BuilderResult,
	err error,
	cacheHit bool,
	fullRebuild bool,
	hotRebuild bool,
) error {
	rebuildReason := o.rebuildReasonSnapshot()
	if err == nil {
		if result != nil {
			if err := result.Validate(); err != nil {
				le.WithError(err).Debug("skipping world-backed manifest build result")
			} else if err := o.c.storeManifestBuildResult(ctx, le, result); err != nil {
				resultPromise.SetResult(nil, err)
				o.prevErr = err
				return err
			}
		}
		resultPromise.SetResult(result, nil)
		o.prevResult = result
		o.prevErr = nil
		o.c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
			State:                   ManifestBuilderLifecycleStateDone,
			CacheHit:                cacheHit,
			FullRebuild:             fullRebuild,
			HotRebuild:              hotRebuild,
			DependencyRebuildReason: rebuildReason,
			Summary:                 "build complete",
		})
		return nil
	}

	resultPromise.SetResult(nil, err)
	o.prevErr = err
	o.c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
		State:                   ManifestBuilderLifecycleStateError,
		FullRebuild:             fullRebuild,
		HotRebuild:              hotRebuild,
		DependencyRebuildReason: rebuildReason,
		Summary:                 "build failed",
		Error:                   err.Error(),
	})
	return nil
}

func (a *manifestBuildAttempt) restart(reason string) {
	var cancel context.CancelFunc
	a.owner.mtx.Lock()
	a.owner.rebuildReason = reason
	if a.owner.current == a && !a.restarted {
		a.restarted = true
		cancel = a.cancel
	}
	a.owner.mtx.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (a *manifestBuildAttempt) wasRestarted() bool {
	a.owner.mtx.Lock()
	defer a.owner.mtx.Unlock()
	return a.restarted
}

func (a *manifestBuildAttempt) release() {
	a.cancel()
	a.owner.mtx.Lock()
	if a.owner.current == a {
		a.owner.current = nil
	}
	a.owner.mtx.Unlock()
}
