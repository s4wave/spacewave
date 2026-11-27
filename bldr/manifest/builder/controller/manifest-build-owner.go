//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/promise"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
)

// manifestBuildOwner owns the manifest build lifecycle: it serializes
// attempts, tracks the previous result and rebuild reason, and publishes
// results to the world state.
//
// Each attempt installs a fresh result promise and moves through these states:
//
//   - building: BuildManifest and the result store run under the attempt
//     context.
//   - canceled: a newer change, a source edit or a sub-manifest result,
//     restarted the attempt. Its promise stays unresolved and the loop begins
//     a new attempt on the newest input, whose promise replaces it.
//   - built or failed: the promise resolves with the result or the build
//     error. A watching controller then waits for the next change. A failed
//     build keeps the previous result's inputs watched.
//
// A promise therefore resolves only with a finished build, never with the
// cancellation of one, and followers move to the replacement promise.
type manifestBuildOwner struct {
	builderConfig *bldr_manifest_builder.BuilderConfig
	resultPromise *promise.PromiseContainer[*bldr_manifest_builder.BuilderResult]

	prevResult   *bldr_manifest_builder.BuilderResult
	prevErr      error
	changedFiles []*bldr_manifest_builder.InputManifest_File

	mtx           sync.Mutex
	rebuildReason string
	current       *manifestBuildAttempt
}

// manifestBuildAttempt is one in-flight build pass. A restart cancels the
// current attempt so the loop can begin a new one with fresh inputs.
type manifestBuildAttempt struct {
	owner     *manifestBuildOwner
	ctx       context.Context
	cancel    context.CancelFunc
	restarted bool
}

// newManifestBuildOwner constructs a manifest build owner that installs each
// attempt's promise in resultPromise.
func newManifestBuildOwner(
	resultPromise *promise.PromiseContainer[*bldr_manifest_builder.BuilderResult],
	builderConfig *bldr_manifest_builder.BuilderConfig,
) *manifestBuildOwner {
	return &manifestBuildOwner{
		builderConfig: builderConfig,
		resultPromise: resultPromise,
	}
}

// beginAttempt registers a new build attempt as the current one,
// canceling any prior attempt.
func (o *manifestBuildOwner) beginAttempt(ctx context.Context) *manifestBuildAttempt {
	// Create the attempt with its own cancelable context.
	attemptCtx, cancel := context.WithCancel(ctx)
	attempt := &manifestBuildAttempt{
		owner:  o,
		ctx:    attemptCtx,
		cancel: cancel,
	}

	// Install the attempt as the current one.
	o.mtx.Lock()
	o.current = attempt
	o.mtx.Unlock()

	return attempt
}

// nextResultPromise installs a fresh result promise on the controller and
// returns it for this attempt.
func (o *manifestBuildOwner) nextResultPromise() *promise.Promise[*bldr_manifest_builder.BuilderResult] {
	resultPromise := promise.NewPromise[*bldr_manifest_builder.BuilderResult]()
	o.resultPromise.SetPromise(resultPromise)
	return resultPromise
}

// buildArgs builds the manifest build arguments from the previous result
// and any changed files, clearing the changed-file set.
func (o *manifestBuildOwner) buildArgs() *bldr_manifest_builder.BuildManifestArgs {
	// Each watch result is a newer selectable revision. The shared
	// configuration stays unchanged for concurrent child builders and
	// startup validation.
	config := o.builderConfig.CloneVT()
	if previous := o.prevResult.GetManifest().GetMeta(); previous != nil && previous.GetRev() >= config.GetManifestMeta().GetRev() {
		config.ManifestMeta.Rev = previous.GetRev() + 1
	}

	// Assemble the build arguments and clear the changed-file set.
	args := &bldr_manifest_builder.BuildManifestArgs{
		BuilderConfig:     config,
		PrevBuilderResult: o.prevResult,
		ChangedFiles:      o.changedFiles,
	}
	o.changedFiles = nil
	return args
}

// rebuildFlags reports the full-rebuild and hot-rebuild flags for a nil
// result.
func (o *manifestBuildOwner) rebuildFlags(result *bldr_manifest_builder.BuilderResult) (bool, bool) {
	if result != nil {
		return false, false
	}
	return o.prevResult == nil, o.prevResult != nil
}

// setChangedFiles records the changed input files as the rebuild reason.
func (o *manifestBuildOwner) setChangedFiles(changedFiles []*bldr_manifest_builder.InputManifest_File) {
	o.changedFiles = changedFiles
	o.setRebuildReason(changedFilesSummary(len(changedFiles)))
}

// rebuildReasonSnapshot returns the current rebuild reason.
func (o *manifestBuildOwner) rebuildReasonSnapshot() string {
	// Read the rebuild reason under the owner mutex.
	o.mtx.Lock()
	defer o.mtx.Unlock()
	return o.rebuildReason
}

// setRebuildReason records why the next build is needed.
func (o *manifestBuildOwner) setRebuildReason(reason string) {
	// Write the rebuild reason under the owner mutex.
	o.mtx.Lock()
	o.rebuildReason = reason
	o.mtx.Unlock()
}

// recordResult records a finished build as the previous result. A failed
// build keeps the previous result, so its inputs stay watched.
func (o *manifestBuildOwner) recordResult(result *bldr_manifest_builder.BuilderResult, err error) {
	if err == nil {
		o.prevResult = result
	}
	o.prevErr = err
}

// inputFiles returns the input files of the previous result, which a
// watching controller watches for changes.
func (o *manifestBuildOwner) inputFiles() []*bldr_manifest_builder.InputManifest_File {
	return o.prevResult.GetInputManifest().GetFiles()
}

// restart cancels the attempt so its loop begins a new pass, recording
// reason. The first restart takes effect. Subsequent restarts are ignored.
func (a *manifestBuildAttempt) restart(reason string) {
	// Record the reason and claim the first restart under the owner mutex.
	var cancel context.CancelFunc
	a.owner.mtx.Lock()
	a.owner.rebuildReason = reason
	if a.owner.current == a && !a.restarted {
		a.restarted = true
		cancel = a.cancel
	}
	a.owner.mtx.Unlock()

	// Cancel the context outside the mutex when this attempt won the restart.
	if cancel != nil {
		cancel()
	}
}

// canceled reports whether the attempt ended: restarted, released or
// canceled with its parent context.
func (a *manifestBuildAttempt) canceled() bool {
	return a.ctx.Err() != nil
}

// done returns a channel that closes when the attempt ends.
func (a *manifestBuildAttempt) done() <-chan struct{} {
	return a.ctx.Done()
}

// wasRestarted reports whether this attempt was restarted.
func (a *manifestBuildAttempt) wasRestarted() bool {
	// Read the restarted flag under the owner mutex.
	a.owner.mtx.Lock()
	defer a.owner.mtx.Unlock()
	return a.restarted
}

// release cancels the attempt and clears it as the current attempt.
func (a *manifestBuildAttempt) release() {
	// Cancel the attempt and clear it as the current attempt.
	a.cancel()
	a.owner.mtx.Lock()
	if a.owner.current == a {
		a.owner.current = nil
	}
	a.owner.mtx.Unlock()
}
