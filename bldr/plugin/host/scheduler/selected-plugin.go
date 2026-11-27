package plugin_host_scheduler

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/sirupsen/logrus"
)

// errNoAdmissionPeer is the error for refusing a load that specifies manifest
// roots without the peer whose approvals decide them.
var errNoAdmissionPeer = errors.New("load names manifests without a peer to admit them")

// errNoApprovalOwner is the error for refusing a load whose root the trust set
// did not admit on a host with no approvals owner, where nothing else can ever
// admit the root.
var errNoApprovalOwner = errors.New("host has no approval owner to admit this root")

// executionReference identifies a candidate and its registration startup mode.
type executionReference struct {
	args     *executePluginArgs
	prepared bool
}

// newExecution constructs one immutable worker without manifest-selection loops.
func (t *pluginInstance) newExecution(key executionReference) (keyed.Routine, *pluginInstance) {
	// Build a physical worker keyed by the candidate's manifest root.
	root := manifestRootKey(key.args.manifestSnapshot.GetManifestRef())
	worker := newPluginState(t.c, t.le, t.pluginID, t.instanceKey+"/generation/"+root, t.manifestRoot)
	worker.bindingKey = t.bindingKey
	worker.physical = true
	worker.prepared = key.prepared
	worker.onLoadState = func(state plugin.PluginLoadState) {
		t.pluginUpdateMtx.Lock()
		defer t.pluginUpdateMtx.Unlock()
		if t.activeExecution == worker {
			t.updatePluginLoadState(func(plugin.PluginLoadState) plugin.PluginLoadState { return state })
		}
	}
	return func(ctx context.Context) error { return worker.execPlugin(ctx, key.args) }, worker
}

// execSelectedPlugin recovers a cold installation with its retained artifacts.
// An admitted worker survives failed replacements without restarting or falling
// back. Manifests that fail startup with a protocol mismatch are rejected for
// this binding, and World selection repeats without them.
func (t *pluginInstance) execSelectedPlugin(ctx context.Context, args *executePluginArgs) error {
	// Try the selection, then its retained fallbacks while no worker runs. A
	// protocol mismatch rejects that manifest on this host and moves on.
	candidates := []*executePluginArgs{args}
	if args != nil {
		candidates = append(candidates, args.fallbacks...)
	}
	candidates, err := t.admittedCandidates(ctx, candidates)
	if err != nil {
		return err
	}
	var protocolErr *plugin_host.StartupProtocolError
	for i, candidate := range candidates {
		if i != 0 && (err == nil || ctx.Err() != nil || t.runningPluginCtr.GetValue() != nil) {
			break
		}
		err = t.execSelectedCandidate(ctx, candidate, i != 0)
		if errors.As(err, &protocolErr) {
			t.incompatibleManifests.Store(manifestRootKey(candidate.manifestSnapshot.GetManifestRef()), struct{}{})
		}
	}
	if !errors.As(err, &protocolErr) {
		return err
	}

	// A proven protocol mismatch is terminal for these candidates.
	// Returning nil stops backoff retries. World selection repeats without
	// the rejected roots.
	t.stopStartupWaitBudget()
	if t.runningPluginCtr.GetValue() == nil {
		t.finishExecution(err)
	}
	if args != nil && args.installation == nil {
		t.manifestSelectionFingerprint.Store(nil)
		t.watchWorldManifestRoutine.RestartRoutine()
	}
	return nil
}

// manifestRootKey identifies a manifest's executable content across buckets.
func manifestRootKey(ref *bucket.ObjectRef) string {
	return ref.GetRootRef().GetHash().MarshalString()
}

// incompatibleManifest reports whether the manifest failed startup on this host.
func (t *pluginInstance) incompatibleManifest(ref *bucket.ObjectRef) bool {
	_, ok := t.incompatibleManifests.Load(manifestRootKey(ref))
	return ok
}

// activationProbeSlow is how long a running worker may take to answer
// Activation.Check before the plugin's swap state says the plugin waits on
// the worker. Tests shorten activationProbeSlow.
var activationProbeSlow = 5 * time.Second

// probeActivation asks the running worker whether it supports prepared
// activation. It waits for an answer, since a healthy worker can be slow: the
// check runs on the plugin's own event loop, behind any long task, and a
// background tab may pause it. Each outcome ends the probe:
//   - answered: prepared, the candidate starts beside the worker.
//   - absent method: unprepared, the candidate replaces the worker in place.
//   - worker gone: unprepared, nothing is left to keep.
//   - transport error: returned and recorded. The worker is preserved.
//
// While no answer has come after activationProbeSlow, the swap state is
// waiting. The answer returns the swap state to preparing.
//
// Go and JavaScript RPC servers report an absent method differently.
func (t *pluginInstance) probeActivation(ctx context.Context, current plugin.RunningPlugin, candidate swapCandidate) (bool, error) {
	// Say the swap waits on the worker once it has been slow.
	sentAt := timestamp.Now()
	recorded := make(chan struct{})
	slow := time.AfterFunc(activationProbeSlow, func() {
		defer close(recorded)
		t.le.WithFields(logrus.Fields{
			"plugin-id":    t.pluginID,
			"instance-key": t.instanceKey,
			"waited":       activationProbeSlow.String(),
		}).Warn("running plugin has not answered activation check; waiting")
		t.setSwapWaiting(ctx, candidate, sentAt)
	})

	// Wait for the answer, then leave the waiting state once it was written.
	_, err := plugin.NewSRPCActivationClient(current.GetRpcClient()).Check(ctx, &plugin.CheckActivationRequest{})
	if !slow.Stop() {
		<-recorded
		t.setSwapPreparing(ctx, candidate)
	}

	// Map the answer to the replacement mode.
	switch {
	case err == nil:
		return true, nil
	case err.Error() == srpc.ErrUnimplemented.Error(),
		err.Error() == "not found: bldr.plugin.Activation/Check",
		plugin.IsWorkerGone(err):
		return false, nil
	case ctx.Err() != nil:
		return false, ctx.Err()
	default:
		return false, err
	}
}

// execSelectedCandidate prepares a candidate while retaining the admitted
// worker. Legacy plugins retain their in-place replacement contract. Typed
// plugins publish their complete registration scope only after startup
// succeeds.
func (t *pluginInstance) execSelectedCandidate(ctx context.Context, args *executePluginArgs, recovery bool) (rerr error) {
	// Reject a candidate the binding no longer accepts.
	if !t.acceptsManifest(args) {
		return context.Canceled
	}

	// Record the attempt's failure unless it was canceled.
	defer func() {
		if rerr != nil && ctx.Err() == nil {
			t.c.recordPluginStatusError(t.pluginID, t.instanceKey, "prepare plugin", rerr)
		}
	}()

	// Wait for the root's admission before any host starts or file is served.
	// A running worker is left alone, so an admitted root keeps running while
	// a new root waits.
	if admitted, err := t.awaitAdmission(ctx, args); err != nil || !admitted {
		return err
	}
	if args.serveAssets {
		return t.serveAssets(ctx, args)
	}

	// Keep an admitted worker whose own candidate is selected again, as
	// when a released installation returns the binding to it. One that
	// finished startup is supervised again. One still starting finishes
	// like a new one.
	if worker, readyAt := t.admittedWorker(args); worker != nil {
		if !readyAt.IsZero() {
			t.settleAdmitted(ctx, worker)
			return t.superviseExecution(ctx, worker, readyAt)
		}
		t.setSwapPreparing(ctx, newSwapCandidate(args))
		if _, err := waitStartup(ctx, worker); err != nil {
			if ctx.Err() == nil {
				t.clearExecution(worker)
			}
			return err
		}
		return t.runAdmitted(ctx, worker, args, recovery)
	}

	// Publish the swap state: preparing now, failed when the attempt fails.
	candidate := newSwapCandidate(args)
	t.setSwapPreparing(ctx, candidate)
	defer func() {
		if rerr != nil && !errors.Is(rerr, context.Canceled) {
			t.setSwapFailed(ctx, candidate, "prepare plugin: "+rerr.Error())
		}
	}()
	if args.pluginHost == nil {
		// A host change selects the candidate again. Retrying finds no
		// host.
		err := errors.New("installed plugin has no compatible host")
		t.c.recordPluginStatusError(t.pluginID, t.instanceKey, "prepare plugin", err)
		t.setSwapFailed(ctx, candidate, "prepare plugin: "+err.Error())
		return nil
	}

	// Probe the current worker's declared admission contract. A prepared
	// worker stays until the candidate activates. Otherwise the worker is
	// replaced in place.
	prepared := false
	if current := t.runningPluginCtr.GetValue(); current != nil && t.manifestRoot == "" {
		var err error
		prepared, err = t.probeActivation(ctx, current, candidate)
		if err != nil {
			return err
		}
	}
	if !prepared {
		t.clearExecution(nil)
	}

	// The logical binding owns workers beyond this selection attempt. Until
	// admission, canceling or failing this attempt releases only the candidate.

	// Register the candidate as an execution and admit it when unprepared.
	ref, worker, _ := t.executions.AddKeyRef(executionReference{args: args, prepared: prepared})
	admitted := false
	defer func() {
		if !admitted {
			ref.Release()
		}
	}()
	if !prepared {
		admitted = t.admitExecution(ctx, worker, ref.Release, args)
		if !admitted {
			return context.Canceled
		}
	}

	// Wait for startup to complete or fail before activation.
	state, err := waitStartup(ctx, worker)
	if err != nil {
		if admitted && ctx.Err() == nil {
			t.clearExecution(worker)
		}
		return err
	}

	// Activate only a ready candidate. Its attached handlers and immutable viewer
	// URLs are already usable before the family RPC alias changes.
	if prepared {
		if !t.acceptsManifest(args) {
			return context.Canceled
		}
		activation := plugin.NewSRPCActivationClient(state.GetRpcClient())
		if _, err := activation.Activate(ctx, &plugin.ActivatePluginRequest{}); err != nil {
			return err
		}
		admitted = t.admitExecution(ctx, worker, ref.Release, args)
		if !admitted {
			return context.Canceled
		}
	}

	return t.runAdmitted(ctx, worker, args, recovery)
}

// waitStartup waits for an admitted or prepared worker to finish startup and
// returns its load state, or the error that ended its startup.
func waitStartup(ctx context.Context, worker *pluginInstance) (plugin.PluginLoadState, error) {
	return worker.pluginLoadStateCtr.WaitValueWithValidator(ctx, func(state plugin.PluginLoadState) (bool, error) {
		if state.GetInitialCapabilityRegistrationState() == plugin.InitialCapabilityRegistrationFailed {
			if err := state.GetStartupError(); err != nil {
				return false, err
			}
			return false, errors.New("plugin exited before completing startup")
		}
		return state.GetRunningPlugin() != nil, nil
	}, nil)
}

// runAdmitted publishes an admitted worker that finished startup, marks it
// ready and supervises it until it exits.
func (t *pluginInstance) runAdmitted(ctx context.Context, worker *pluginInstance, args *executePluginArgs, recovery bool) error {
	// Publish the admitted manifest root and clear startup bookkeeping.
	t.emitPluginManifestRoot(args.manifestSnapshot.GetManifestRef().GetRootRef().GetHash().MarshalString())
	if !recovery {
		t.c.clearPluginStatusError(t.pluginID, t.instanceKey)
	}
	t.stopStartupWaitBudget()

	// Mark the worker ready and publish the settled swap.
	readyAt, _ := t.settleAdmitted(ctx, worker)
	return t.superviseExecution(ctx, worker, readyAt)
}

// admittedWorker returns the admitted worker if the worker runs args itself,
// and the time the worker finished startup (zero while still starting).
// Otherwise, admittedWorker returns nil.
func (t *pluginInstance) admittedWorker(args *executePluginArgs) (*pluginInstance, time.Time) {
	t.pluginUpdateMtx.Lock()
	defer t.pluginUpdateMtx.Unlock()
	if t.activeArgs != args {
		return nil, time.Time{}
	}
	return t.activeExecution, t.activeReadyAt
}

// settleAdmitted marks the worker ready, keeping an earlier ready time, and
// publishes the settled swap. Both happen under pluginUpdateMtx and only while
// the worker is still admitted, so a worker cleared meanwhile is never shown
// as settled. It returns the ready time and whether the worker was admitted.
func (t *pluginInstance) settleAdmitted(ctx context.Context, worker *pluginInstance) (time.Time, bool) {
	// A worker cleared since the caller saw it is not settled.
	t.pluginUpdateMtx.Lock()
	defer t.pluginUpdateMtx.Unlock()
	if t.activeExecution != worker {
		return time.Now(), false
	}
	if t.activeReadyAt.IsZero() {
		t.activeReadyAt = time.Now()
	}
	t.setSwapSettled(ctx, t.activeReadyAt)
	return t.activeReadyAt, true
}

// superviseExecution waits for the admitted worker, ready since admittedAt,
// to report a failed registration, then releases it and reports its exit.
// The worker's callback keeps public state current even while another
// candidate is preparing.
func (t *pluginInstance) superviseExecution(ctx context.Context, worker *pluginInstance, admittedAt time.Time) error {
	// Wait for the admitted worker to report a failed registration.
	_, err := worker.pluginLoadStateCtr.WaitValueWithValidator(ctx, func(state plugin.PluginLoadState) (bool, error) {
		return state.GetInitialCapabilityRegistrationState() == plugin.InitialCapabilityRegistrationFailed, nil
	}, nil)
	if err != nil {
		return err
	}
	t.clearExecution(worker)

	// A worker that outlived the longest restart delay restarts promptly. One
	// that crashes sooner keeps growing the delay like a startup failure.
	if time.Since(admittedAt) >= execBackoffMaxInterval {
		t.execBackoff.Reset()
	}
	return errors.New("admitted plugin exited")
}

// admitExecution switches the family alias and filesystem access to a worker.
// Its load-state lock precedes pluginUpdateMtx, matching the worker callback.
// The scheduler's status mutex comes after both. ctx is the selection
// attempt's: a superseded or released candidate is canceled under
// pluginUpdateMtx, so its attempt admits nothing afterwards.
func (t *pluginInstance) admitExecution(ctx context.Context, worker *pluginInstance, release func(), args *executePluginArgs) bool {
	// Swap the admitted worker under the load-state lock.
	var previous func()
	var admitted bool
	worker.pluginLoadStateCtr.SwapValue(func(state plugin.PluginLoadState) plugin.PluginLoadState {
		// Admit the worker only while the binding is open, the attempt is
		// current and the binding accepts the manifest.
		t.pluginUpdateMtx.Lock()
		defer t.pluginUpdateMtx.Unlock()
		if t.executionsClosed || ctx.Err() != nil || !t.acceptsManifest(args) {
			return state
		}
		admitted = true
		previous = t.releaseExecution
		t.releaseExecution = release

		// Record the worker and the candidate it runs. The worker is
		// not ready yet.
		t.activeExecution = worker
		t.activeArgs = args
		t.activeReadyAt = time.Time{}

		// Switch the family alias and file access to the new worker.
		t.ensureAccessProviders()
		t.distAccess.SetCurrent(worker.distAccess.AccessUnixFS)
		t.assetsAccess.SetCurrent(worker.assetsAccess.AccessUnixFS)
		t.updatePluginLoadState(func(plugin.PluginLoadState) plugin.PluginLoadState { return state })

		// Publish the running manifest in the same critical section, so a
		// concurrent clear cannot be overwritten by a stale admission.
		t.setRunningManifest(args)
		return state
	})

	// Release the previously admitted worker after the swap commits.
	if previous != nil {
		previous()
	}
	return admitted
}

// restart ends the admitted worker and runs the selection attempt again at
// once, skipping any restart delay. The new attempt finds nothing running, so
// it starts the selected manifest in place. Restarting is the way out of a swap
// waiting on a worker that neither answers its activation check nor exits:
// ending the worker first means the restarted attempt cannot probe it again.
func (t *pluginInstance) restart() {
	t.clearExecution(nil)
	t.executePluginRoutine.RestartRoutine()
}

// closeExecutions prevents late admission after the logical binding closes.
func (t *pluginInstance) closeExecutions() {
	t.pluginUpdateMtx.Lock()
	t.executionsClosed = true
	t.pluginUpdateMtx.Unlock()
	t.clearExecution(nil)
}

// clearExecution releases the admitted worker and clears the logical binding.
func (t *pluginInstance) clearExecution(expected *pluginInstance) {
	// Return unchanged when no worker or a different worker is admitted.
	t.pluginUpdateMtx.Lock()
	if t.activeExecution == nil || expected != nil && t.activeExecution != expected {
		t.pluginUpdateMtx.Unlock()
		return
	}

	// Clear the admitted worker and block file access until the next admission.
	release := t.releaseExecution
	t.releaseExecution = nil
	t.activeExecution = nil
	t.activeArgs = nil
	t.ensureAccessProviders()
	t.distAccess.SetBlocked()
	t.assetsAccess.SetBlocked()
	t.beginInitialCapabilityRegistration()

	// Publish that nothing runs before another admission can take the lock.
	t.setRunningManifest(nil)
	t.clearReleasedSwapLocked()
	t.pluginUpdateMtx.Unlock()

	// Release the replaced worker outside the update lock.
	if release != nil {
		release()
	}
}

// admissionPeer returns the peer whose approvals decide the candidate's root,
// and whether the root needs admission. A removal, with no manifest, needs
// none. A scheduler that requires admission serves one peer and asks for its
// configured peer about every root, so a load cannot specify another. Any
// other scheduler asks only about the roots a load specifies, for the load's
// peer. The roots its own catalog selects run as before.
func (t *pluginInstance) admissionPeer(args *executePluginArgs) (string, bool) {
	switch {
	case args == nil || args.manifestSnapshot == nil:
		return "", false
	case t.c.conf.GetRequireAdmission():
		return t.c.conf.GetPeerId(), true
	case args.installation != nil:
		return args.installation.peerID, true
	default:
		return "", false
	}
}

// awaitsOwner reports whether a root the resolvers did not admit may still be
// admitted later: the host answers admission itself, or an approvals owner
// can admit it. Otherwise the root is refused rather than left waiting.
func (t *pluginInstance) awaitsOwner() bool {
	return t.c.conf.GetRequireAdmission() || t.c.conf.GetAdmissionPluginId() != ""
}

// admittedCandidates returns the candidates that may run, keeping their order:
// each is decided by its own peer, and passes when it needs no admission or its
// root is admitted. Before each root is read, the swap state says its approval
// is being checked, so a read that does not answer shows what it waits for, and
// an admitted root never shows as awaiting approval. With no candidate passing,
// admittedCandidates waits until a waiting root is admitted, so an unadmitted
// preferred candidate never holds back an admitted fallback. A candidate that
// specifies a root without a peer never runs, nor does an unadmitted one on a
// host with no approvals owner, and one whose read failed is skipped. When none
// passes and a read or wait failed, the failed swap state records that error
// and the load retries instead of showing a wait that never ends.
func (t *pluginInstance) admittedCandidates(ctx context.Context, candidates []*executePluginArgs) ([]*executePluginArgs, error) {
	for {
		// Sort the candidates into those that pass now, those that wait for
		// admission, the first that can never be admitted and the first that
		// failed.
		var admitted, waiting []*executePluginArgs
		var refused, failed *executePluginArgs
		var refusedErr, failedErr error
		for _, candidate := range candidates {
			peerID, ask := t.admissionPeer(candidate)
			if !ask {
				admitted = append(admitted, candidate)
				continue
			}
			if peerID == "" {
				if refused == nil {
					refused, refusedErr = candidate, errNoAdmissionPeer
				}
				continue
			}
			t.setSwapCheckingApproval(ctx, newSwapCandidate(candidate), peerID)
			ok, err := t.rootAdmitted(ctx, peerID, candidate)
			switch {
			case ctx.Err() != nil:
				return nil, context.Cause(ctx)
			case err != nil:
				if failed == nil {
					failed, failedErr = candidate, err
				}
			case ok:
				admitted = append(admitted, candidate)
			case t.awaitsOwner():
				waiting = append(waiting, candidate)
			case refused == nil:
				refused, refusedErr = candidate, errNoApprovalOwner
			}
		}
		if len(admitted) != 0 {
			return admitted, nil
		}
		if failed != nil {
			return nil, t.failAdmission(ctx, failed, failedErr)
		}

		// Refuse a load none of whose roots can ever be admitted.
		if len(waiting) == 0 {
			t.setSwapFailed(ctx, newSwapCandidate(refused), "prepare plugin: "+refusedErr.Error())
			return nil, nil
		}

		// Wait until any root is admitted, then check the order again.
		if failed, err := t.awaitAnyAdmission(ctx, waiting); err != nil {
			return nil, t.failAdmission(ctx, failed, err)
		}
	}
}

// failAdmission publishes that reading or waiting for the candidate's
// admission failed with err and returns err. setSwap drops the write when ctx
// ended. The execute routine retries the failed load with backoff and fills
// the retry time.
func (t *pluginInstance) failAdmission(ctx context.Context, args *executePluginArgs, err error) error {
	t.setSwapFailed(ctx, newSwapCandidate(args), "await plugin approval: "+err.Error())
	return err
}

// rootAdmitted reports whether the candidate's root is admitted for peerID
// once every admission resolver has answered.
func (t *pluginInstance) rootAdmitted(ctx context.Context, peerID string, args *executePluginArgs) (bool, error) {
	// Ask once, returning when the resolvers go idle with or without a value.
	root := manifestRootKey(args.manifestSnapshot.GetManifestRef())
	_, _, ref, err := bus.ExecWaitValue[plugin.AdmitPluginRootValue](
		ctx,
		t.c.bus,
		plugin.NewAdmitPluginRoot(peerID, t.pluginID, root),
		bus.ReturnIfIdle(true),
		nil,
		nil,
	)
	if err != nil || ref == nil {
		return false, err
	}
	ref.Release()
	return true, nil
}

// admissionResult is one wait's answer: nil on an admission, or the error that
// ended the wait for candidate.
type admissionResult struct {
	candidate *executePluginArgs
	err       error
}

// awaitAnyAdmission waits until the root of any waiting candidate is admitted
// for that candidate's peer, publishing that the first awaits approval
// meanwhile. It returns on the first wait's answer: nil on an admission, or
// the candidate whose wait failed and its error, so the caller fails the
// load, none of whose roots passed, instead of leaving it to the waits that
// remain.
func (t *pluginInstance) awaitAnyAdmission(ctx context.Context, waiting []*executePluginArgs) (*executePluginArgs, error) {
	// Say what the plugin waits for before waiting.
	peerID, _ := t.admissionPeer(waiting[0])
	t.setSwapAwaitingApproval(ctx, newSwapCandidate(waiting[0]), peerID)

	// Wait on every root at once. The first result cancels the rest.
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan admissionResult, len(waiting))
	for _, candidate := range waiting {
		peerID, _ := t.admissionPeer(candidate)
		root := manifestRootKey(candidate.manifestSnapshot.GetManifestRef())
		go func() {
			_, err := plugin.WaitPluginRootAdmitted(waitCtx, t.c.bus, peerID, t.pluginID, root)
			results <- admissionResult{candidate: candidate, err: err}
		}()
	}
	select {
	case <-ctx.Done():
		return waiting[0], context.Cause(ctx)
	case res := <-results:
		return res.candidate, res.err
	}
}

// awaitAdmission blocks until the bus admits the candidate's manifest root,
// when the root needs admission, and reports whether the candidate may start.
// The swap state says the root's approval is being checked until the resolvers
// answer, and that it awaits approval once they answered without admitting it.
// A root no resolver admits waits until ctx ends. A load that specifies no
// peer, or whose root is not admitted on a host with no approvals owner, can
// never be admitted: awaitAdmission returns false with a nil error and the swap
// state failed, as admittedCandidates refuses the load, so the execute routine
// does not retry the load.
func (t *pluginInstance) awaitAdmission(ctx context.Context, args *executePluginArgs) (bool, error) {
	// Skip a root the scheduler runs without asking.
	peerID, ask := t.admissionPeer(args)
	if !ask {
		return true, nil
	}
	candidate := newSwapCandidate(args)
	if peerID == "" {
		t.setSwapFailed(ctx, candidate, "prepare plugin: "+errNoAdmissionPeer.Error())
		return false, nil
	}

	// Check the root's approval, saying so until the resolvers answer.
	root := manifestRootKey(args.manifestSnapshot.GetManifestRef())
	le := t.le.WithFields(logrus.Fields{
		"plugin-id":     t.pluginID,
		"manifest-root": root,
	})
	t.setSwapCheckingApproval(ctx, candidate, peerID)
	ok, err := t.rootAdmitted(ctx, peerID, args)
	if err != nil {
		return false, t.failAdmission(ctx, args, err)
	}
	if ok {
		return true, nil
	}
	if !t.awaitsOwner() {
		t.setSwapFailed(ctx, candidate, "prepare plugin: "+errNoApprovalOwner.Error())
		return false, nil
	}

	// Wait for a resolver to admit the root.
	le.Debug("waiting for plugin root admission")
	t.setSwapAwaitingApproval(ctx, candidate, peerID)
	src, err := plugin.WaitPluginRootAdmitted(ctx, t.c.bus, peerID, t.pluginID, root)
	if err != nil {
		return false, t.failAdmission(ctx, args, err)
	}
	le.WithField("admit-source", src).Debug("plugin root admitted")
	return true, nil
}
