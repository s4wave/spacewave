package plugin_host_scheduler

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/callback"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_trust "github.com/s4wave/spacewave/bldr/manifest/trust"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_admit "github.com/s4wave/spacewave/bldr/plugin/admit"
	plugin_approval "github.com/s4wave/spacewave/core/plugin/approval"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
	s4wave_plugin_approval "github.com/s4wave/spacewave/sdk/plugin/approval"
	"github.com/sirupsen/logrus"
)

// candidateActivation exposes the ready candidate's admission boundary.
type candidateActivation struct {
	activated chan struct{}
	// answer, when set before the check, holds Check until it yields the
	// call's result, like a worker that is busy or leaves.
	answer chan error
}

// Check declares support for staged replacement, after receiving from answer
// when set.
func (a *candidateActivation) Check(ctx context.Context, _ *plugin.CheckActivationRequest) (*plugin.CheckActivationResponse, error) {
	if a.answer != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-a.answer:
			if err != nil {
				return nil, err
			}
		}
	}
	return &plugin.CheckActivationResponse{}, nil
}

// Activate records admission after the worker reports startup complete.
func (a *candidateActivation) Activate(context.Context, *plugin.ActivatePluginRequest) (*plugin.ActivatePluginResponse, error) {
	close(a.activated)
	return &plugin.ActivatePluginResponse{}, nil
}

// TestSelectedPluginRetainsAdmittedWorker exercises the actual selection owner
// while worker startup is blocked, fails, succeeds, or is superseded.
func TestSelectedPluginRetainsAdmittedWorker(t *testing.T) {
	// Retain worker lifetime independently from each candidate-selection request.
	// Bind a bounded context, logger, and controller with one instance.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	c := &Controller{
		le: le, conf: &Config{},
	}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
	c.setPluginStatus("colors", "", plugin.PluginState_PluginState_REQUESTED)

	// waitExecution waits for the colors execution status to satisfy ok and
	// logs the swap state it reached.
	waitExecution := func(phase string, ok func(*plugin.PluginExecutionStatus) bool) {
		t.Helper()
		snap, err := c.pluginStatus.WaitValueWithValidator(ctx, func(snap *plugin.PluginStatusSnapshot) (bool, error) {
			for _, st := range snap.GetExecutions() {
				if st.GetPluginId() == "colors" && ok(st) {
					return true, nil
				}
			}
			return false, nil
		}, nil)
		if err != nil {
			t.Fatalf("%s: %v", phase, err)
		}
		for _, st := range snap.GetExecutions() {
			t.Logf("%s: running rev %d, swap %s", phase, st.GetRunningManifestRev(), st.GetSwap().MarshalProtoText())
		}
	}

	// Declare the execution record and the channel reporting started workers.
	type execution struct {
		worker     *pluginInstance
		start      chan bool
		closed     chan struct{}
		activation *candidateActivation
	}
	started := make(chan *execution, 1)
	instance.executions = keyed.NewKeyedRefCount(func(key executionReference) (keyed.Routine, *pluginInstance) {
		_, worker := instance.newExecution(key)
		run := &execution{worker: worker, start: make(chan bool, 1), closed: make(chan struct{}), activation: &candidateActivation{activated: make(chan struct{})}}
		return func(ctx context.Context) error {
			// Publish the started execution and wait for the test's verdict.
			defer close(run.closed)
			defer worker.finishExecution(nil)
			started <- run
			select {
			case <-ctx.Done():
				return ctx.Err()
			case success := <-run.start:
				if !success {
					return errors.New("startup failed")
				}
			}

			// Admit the worker through an activation RPC served in-process.
			mux := srpc.NewMux()
			if err := plugin.SRPCRegisterActivation(mux, run.activation); err != nil {
				return err
			}
			worker.updateRpcClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
			worker.finishInitialCapabilityRegistration(true)
			<-ctx.Done()
			return ctx.Err()
		}, worker
	})
	instance.executions.SetContext(ctx, true)
	defer instance.executions.ClearContext()
	defer instance.clearExecution(nil)

	// candidate builds the args selecting the given revision as the world
	// selection does: a ManifestRef and its rev, with no fetched Manifest.
	candidate := func(rev uint64) *executePluginArgs {
		return &executePluginArgs{pluginHost: &testPluginHost{id: "test"}, rev: rev, manifestSnapshot: &manifest.ManifestSnapshot{
			ManifestRef: newTestManifestRef("colors", "js", rev, "candidate").GetManifestRef(),
		}}
	}
	// nextStarted waits for the next worker to start.
	nextStarted := func() *execution {
		t.Helper()
		select {
		case run := <-started:
			return run
		case <-ctx.Done():
			t.Fatal(ctx.Err())
			return nil
		}
	}

	// start launches a candidate at the given revision and waits for it to start.
	start := func(rev uint64) (*execution, context.CancelFunc, <-chan error) {
		// Run selection in the background.
		t.Helper()
		attempt, stop := context.WithCancel(ctx)
		args := candidate(rev)
		done := make(chan error, 1)
		go func() { done <- instance.execSelectedPlugin(attempt, args) }()
		return nextStarted(), stop, done
	}
	wait := func(closed <-chan struct{}) {
		t.Helper()
		select {
		case <-closed:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}

	// The first worker becomes ready and survives cancellation of its selector.
	old, stopOld, doneOld := start(1)
	old.start <- true
	initial, err := instance.runningPluginCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	waitExecution("first admitted", func(st *plugin.PluginExecutionStatus) bool {
		return st.GetRunningManifestRev() == 1 && st.GetSwap().GetSettled() != nil
	})
	stopOld()
	<-doneOld
	select {
	case <-old.closed:
		t.Fatal("canceling selection stopped the admitted worker")
	default:
	}

	// A broken candidate cannot replace the family RPC alias or stop old work.
	failed, stopFailed, doneFailed := start(2)
	defer stopFailed()
	if !failed.worker.prepared || instance.runningPluginCtr.GetValue() != initial {
		t.Fatal("preparing candidate replaced the admitted worker")
	}
	waitExecution("candidate preparing", func(st *plugin.PluginExecutionStatus) bool {
		return st.GetRunningManifestRev() == 1 && st.GetSwap().GetPreparing().GetCandidateManifestRev() == 2
	})
	failed.start <- false
	if err := <-doneFailed; err == nil {
		t.Fatal("failed startup succeeded")
	}
	waitExecution("candidate failed", func(st *plugin.PluginExecutionStatus) bool {
		return st.GetRunningManifestRev() == 1 && st.GetSwap().GetFailed().GetCandidateManifestRev() == 2
	})
	wait(failed.closed)
	if instance.runningPluginCtr.GetValue() != initial {
		t.Fatal("failed candidate removed the admitted worker")
	}

	// Superseding an unfinished candidate releases it without disturbing old work.
	canceled, stopCanceled, doneCanceled := start(3)
	stopCanceled()
	if err := <-doneCanceled; err == nil {
		t.Fatal("canceled candidate succeeded")
	}
	wait(canceled.closed)
	if instance.runningPluginCtr.GetValue() != initial {
		t.Fatal("canceling candidate removed the admitted worker")
	}

	// A ready candidate is activated once. The retired worker's teardown
	// cannot clear the new RPC alias. Releasing the binding ends the
	// remaining worker.
	next, stopNext, doneNext := start(4)
	next.start <- true
	wait(next.activation.activated)

	// The replacement publishes and the retired worker's teardown cannot clear it.
	if _, err := instance.runningPluginCtr.WaitValueChange(ctx, initial, nil); err != nil {
		t.Fatal(err)
	}
	wait(old.closed)
	if instance.runningPluginCtr.GetValue() == nil {
		t.Fatal("retired teardown cleared the replacement")
	}
	waitExecution("replacement admitted", func(st *plugin.PluginExecutionStatus) bool {
		return st.GetRunningManifestRev() == 4 && st.GetSwap().GetSettled() != nil
	})

	stopNext()
	<-doneNext

	// A slow worker is never replaced for slowness: the swap state says the
	// candidate waits on it, and its late answer stages the candidate.
	prevSlow := activationProbeSlow
	activationProbeSlow = 20 * time.Millisecond
	defer func() { activationProbeSlow = prevSlow }()
	waitingStatus := func(want bool) error {
		_, err := c.pluginStatus.WaitValueWithValidator(ctx, func(snap *plugin.PluginStatusSnapshot) (bool, error) {
			for _, st := range snap.GetExecutions() {
				if st.GetPluginId() == "colors" && st.GetSwap().GetWaiting() != nil {
					t.Logf("candidate waiting: running rev %d, swap %s", st.GetRunningManifestRev(), st.GetSwap().MarshalProtoText())
					return want, nil
				}
			}
			return !want, nil
		}, nil)
		return err
	}
	// answer yields result from a's check once the status says it waits.
	answer := func(a *candidateActivation, result error) {
		go func() {
			if waitingStatus(true) == nil {
				a.answer <- result
			}
		}()
	}
	next.activation.answer = make(chan error)
	answer(next.activation, nil)
	slow, stopSlow, doneSlow := start(5)
	if !slow.worker.prepared {
		t.Fatal("slow worker was replaced in place")
	}
	select {
	case <-next.closed:
		t.Fatal("slow worker stopped before the candidate activated")
	default:
	}
	slow.start <- true
	wait(slow.activation.activated)
	wait(next.closed)
	if err := waitingStatus(false); err != nil {
		t.Fatal(err)
	}
	stopSlow()
	<-doneSlow

	// A worker that leaves while the check waits is replaced in place.
	slow.activation.answer = make(chan error)
	answer(slow.activation, plugin.ErrWorkerGone)
	current := instance.runningPluginCtr.GetValue()
	last, stopLast, doneLast := start(6)
	if last.worker.prepared {
		t.Fatal("departed worker was treated as prepared")
	}
	wait(slow.closed)
	last.start <- true
	if _, err := instance.runningPluginCtr.WaitValueChange(ctx, current, nil); err != nil {
		t.Fatal(err)
	}

	stopLast()
	<-doneLast

	// A worker that never answers keeps the swap waiting until a restart
	// ends the worker. The execute routine then starts the candidate in its
	// place.
	if _, err := instance.runningPluginCtr.WaitValue(ctx, nil); err != nil {
		t.Fatal(err)
	}
	last.activation.answer = make(chan error)
	instance.executePluginRoutine.SetContext(ctx, true)
	instance.executePluginRoutine.SetState(candidate(7))
	if err := waitingStatus(true); err != nil {
		t.Fatal(err)
	}
	instance.restart()
	wait(last.closed)
	restarted := nextStarted()
	if restarted.worker.prepared {
		t.Fatal("restart staged the candidate beside the ended worker")
	}
	restarted.start <- true
	waitExecution("restarted", func(st *plugin.PluginExecutionStatus) bool {
		return st.GetRunningManifestRev() == 7 && st.GetSwap().GetSettled() != nil
	})
	instance.executePluginRoutine.ClearContext()

	// Releasing the binding ends the remaining worker.
	instance.closeExecutions()
	wait(restarted.closed)
	if len(instance.executions.GetKeys()) != 0 {
		t.Fatal("released binding retained worker references")
	}
	if instance.admitExecution(t.Context(), last.worker, func() {}, nil) {
		t.Fatal("closed binding admitted a late candidate")
	}
}

// TestFailingPluginRestartBacksOff checks that a plugin failing at start
// restarts after exponentially growing delays.
func TestFailingPluginRestartBacksOff(t *testing.T) {
	// Fail every execution at startup and record when each begins.
	le := logrus.NewEntry(logrus.New())
	c := &Controller{le: le,
		conf: &Config{ExecBackoff: &backoff.Backoff{
			BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
			Exponential: &backoff.Exponential{InitialInterval: 20, Multiplier: 2, MaxInterval: 1000},
		}},
	}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "sample"})
	c.setPluginStatus("sample", "", plugin.PluginState_PluginState_REQUESTED)
	starts := make(chan time.Time, 8)
	instance.executions = keyed.NewKeyedRefCount(func(key executionReference) (keyed.Routine, *pluginInstance) {
		_, worker := instance.newExecution(key)
		return func(context.Context) error {
			// Record the start and fail before completing registration.
			starts <- time.Now()
			worker.beginInitialCapabilityRegistration()
			err := errors.New("wasm error: out of bounds memory access")
			worker.finishExecution(err)
			return err
		}, worker
	})
	instance.executions.SetContext(t.Context(), true)
	t.Cleanup(instance.executions.ClearContext)
	t.Cleanup(func() { instance.clearExecution(nil) })

	// Run the selection through the execute routine that owns the retries.
	instance.executePluginRoutine.SetState(&executePluginArgs{
		pluginHost: &testPluginHost{id: "test"},
		manifestSnapshot: &manifest.ManifestSnapshot{
			ManifestRef: newTestManifestRef("sample", "js", 1, "candidate").GetManifestRef(),
			Manifest:    &manifest.Manifest{Meta: manifest.NewManifestMeta("sample", manifest.BuildType_RELEASE, "js", 1)},
		},
	})
	instance.executePluginRoutine.SetContext(t.Context(), true)
	t.Cleanup(func() { instance.executePluginRoutine.ClearContext() })

	// Each delay is at least the backoff interval, which doubles per failure.
	prev := <-starts
	for i, want := range []time.Duration{20, 40, 80, 160} {
		next := <-starts
		if gap := next.Sub(prev); gap < want*time.Millisecond {
			t.Fatalf("restart %d after %v, want at least %v", i+1, gap, want*time.Millisecond)
		}
		prev = next
	}

	// The failed swap state carries when the routine retries.
	snap, err := c.pluginStatus.WaitValueWithValidator(t.Context(), func(snap *plugin.PluginStatusSnapshot) (bool, error) {
		for _, st := range snap.GetExecutions() {
			if st.GetSwap().GetFailed().GetRetryAt() != nil {
				return true, nil
			}
		}
		return false, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("candidate failed with retry: %s", snap.GetExecutions()[0].GetSwap().MarshalProtoText())
}

// TestSelectedPluginAwaitsAdmission checks that a scheduler requiring
// admission starts no worker for an unadmitted root, keeps the running worker
// while a new root waits, and switches once the new root is admitted.
func TestSelectedPluginAwaitsAdmission(t *testing.T) {
	// Admit roots through the trust controller on a real bus.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	trusted := bldr_manifest_trust.NewSet()
	rel, err := tb.Bus.AddController(ctx, bldr_manifest_trust.NewController(trusted), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	c := &Controller{
		le: le, bus: tb.Bus, conf: &Config{RequireAdmission: true, PeerId: "peer"},
	}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
	c.setPluginStatus("colors", "", plugin.PluginState_PluginState_REQUESTED)

	// Each worker reports its start, then serves activation until released.
	type execution struct {
		closed     chan struct{}
		activation *candidateActivation
	}
	started := make(chan *execution, 1)
	instance.executions = keyed.NewKeyedRefCount(func(key executionReference) (keyed.Routine, *pluginInstance) {
		_, worker := instance.newExecution(key)
		run := &execution{closed: make(chan struct{}), activation: &candidateActivation{activated: make(chan struct{})}}
		return func(ctx context.Context) error {
			defer close(run.closed)
			defer worker.finishExecution(nil)
			started <- run
			mux := srpc.NewMux()
			if err := plugin.SRPCRegisterActivation(mux, run.activation); err != nil {
				return err
			}
			worker.updateRpcClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
			worker.finishInitialCapabilityRegistration(true)
			<-ctx.Done()
			return ctx.Err()
		}, worker
	})
	instance.executions.SetContext(ctx, true)
	defer instance.executions.ClearContext()
	defer instance.clearExecution(nil)

	// candidate selects rev with a manifest root derived from it.
	roots := make(map[uint64]string)
	candidate := func(rev uint64) *executePluginArgs {
		t.Helper()
		h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte{byte(rev)})
		if err != nil {
			t.Fatal(err)
		}
		ref := newTestManifestRef("colors", "js", rev, "candidate").GetManifestRef()
		ref.RootRef = &block.BlockRef{Hash: h}
		roots[rev] = manifestRootKey(ref)
		return &executePluginArgs{pluginHost: &testPluginHost{id: "test"}, rev: rev, manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: ref}}
	}
	// selectRev runs selection of rev in the background.
	selectRev := func(rev uint64) (context.CancelFunc, <-chan error) {
		attempt, stop := context.WithCancel(ctx)
		args := candidate(rev)
		done := make(chan error, 1)
		go func() { done <- instance.execSelectedPlugin(attempt, args) }()
		return stop, done
	}
	// noStart asserts that no worker starts for a while.
	noStart := func(phase string) {
		t.Helper()
		select {
		case <-started:
			t.Fatalf("%s: a worker started for an unadmitted root", phase)
		case <-time.After(200 * time.Millisecond):
		}
	}
	nextStarted := func() *execution {
		t.Helper()
		select {
		case run := <-started:
			return run
		case <-ctx.Done():
			t.Fatal(ctx.Err())
			return nil
		}
	}
	admit := func(rev uint64) {
		trusted.Add(bldr_manifest_trust.Root{PluginID: "colors", ManifestRoot: roots[rev]})
	}

	// An unadmitted root runs nothing. Admitting the root starts the
	// worker.
	stopFirst, doneFirst := selectRev(1)
	noStart("first root")
	admit(1)
	first := nextStarted()
	initial, err := instance.runningPluginCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	stopFirst()
	<-doneFirst

	// A new root waits while the old worker keeps running.
	stopNext, doneNext := selectRev(2)
	defer stopNext()
	noStart("new root")
	select {
	case <-first.closed:
		t.Fatal("a waiting root stopped the running worker")
	default:
	}
	if instance.runningPluginCtr.GetValue() != initial {
		t.Fatal("a waiting root replaced the running worker")
	}

	// Admitting the new root switches to it.
	admit(2)
	next := nextStarted()
	select {
	case <-next.activation.activated:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := instance.runningPluginCtr.WaitValueChange(ctx, initial, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first.closed:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stopNext()
	<-doneNext

	// An unadmitted preferred root does not hold back an admitted fallback.
	got, err := instance.admittedCandidates(ctx, []*executePluginArgs{candidate(3), candidate(1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].rev != 1 {
		t.Fatalf("admitted candidates = %d, want only rev 1", len(got))
	}

	// With none admitted, admitting any root ends the wait.
	waited := make(chan []*executePluginArgs, 1)
	go func() {
		got, _ := instance.admittedCandidates(ctx, []*executePluginArgs{candidate(4), candidate(5)})
		waited <- got
	}()
	select {
	case <-waited:
		t.Fatal("no candidate is admitted, yet the wait ended")
	case <-time.After(200 * time.Millisecond):
	}
	admit(5)
	select {
	case got := <-waited:
		if len(got) != 1 || got[0].rev != 5 {
			t.Fatalf("admitted candidates = %d, want only rev 5", len(got))
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	instance.closeExecutions()
}

// TestSchedulerAdmissionByHost checks which roots each host scheduler runs
// without asking, which it refuses, and for which peer it asks.
func TestSchedulerAdmissionByHost(t *testing.T) {
	// Admit roots through the trust controller on a real bus.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	trusted := bldr_manifest_trust.NewSet()
	rel, err := tb.Bus.AddController(ctx, bldr_manifest_trust.NewController(trusted), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// newInstance builds a plugin instance on a scheduler with conf.
	newInstance := func(conf *Config) (*Controller, *pluginInstance) {
		c := &Controller{
			le: le, bus: tb.Bus, conf: conf,
		}
		_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
		c.setPluginStatus("colors", "", plugin.PluginState_PluginState_REQUESTED)
		return c, instance
	}
	// candidate selects rev, specified by a load for peerID when load is
	// set.
	candidate := func(rev uint64, load bool, peerID string) (*executePluginArgs, string) {
		h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte{0xa0, byte(rev)})
		if err != nil {
			t.Fatal(err)
		}
		ref := newTestManifestRef("colors", "js", rev, "candidate").GetManifestRef()
		ref.RootRef = &block.BlockRef{Hash: h}
		args := &executePluginArgs{pluginHost: &testPluginHost{id: "test"}, rev: rev, manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: ref}}
		if load {
			args.installation = &installedManifests{peerID: peerID}
		}
		return args, manifestRootKey(ref)
	}
	// swapOf returns the instance's published swap state.
	swapOf := func(c *Controller) *plugin.PluginSwap {
		for _, st := range c.pluginStatus.GetValue().GetExecutions() {
			return st.GetSwap()
		}
		return nil
	}
	// admitted runs admittedCandidates and returns how many candidates pass.
	admitted := func(instance *pluginInstance, args *executePluginArgs) int {
		t.Helper()
		got, err := instance.admittedCandidates(ctx, []*executePluginArgs{args})
		if err != nil {
			t.Fatal(err)
		}
		return len(got)
	}
	// awaits asserts that the candidate waits for approval for wantPeer.
	awaits := func(c *Controller, instance *pluginInstance, args *executePluginArgs, wantPeer string) {
		t.Helper()
		waitCtx, stop := context.WithTimeout(ctx, 300*time.Millisecond)
		defer stop()
		got, err := instance.admittedCandidates(waitCtx, []*executePluginArgs{args})
		if err == nil || len(got) != 0 {
			t.Fatalf("unapproved root returned %d candidates, err %v", len(got), err)
		}
		waiting := swapOf(c).GetAwaitingApproval()
		if waiting == nil || waiting.GetPeerId() != wantPeer || waiting.GetCandidateManifestRev() != args.rev {
			t.Fatalf("swap = %s, want awaiting approval for %s", swapOf(c).MarshalProtoText(), wantPeer)
		}
	}

	t.Run("plain load needs no ask", func(t *testing.T) {
		_, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		args, _ := candidate(1, false, "")
		if n := admitted(instance, args); n != 1 {
			t.Fatalf("plain catalog load passed %d candidates, want 1", n)
		}
	})
	t.Run("manifest load with empty peer refused", func(t *testing.T) {
		c, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		args, root := candidate(2, true, "")
		trusted.Add(bldr_manifest_trust.Root{PluginID: "colors", ManifestRoot: root})
		if n := admitted(instance, args); n != 0 {
			t.Fatalf("load with no peer passed %d candidates, want 0", n)
		}
		if swapOf(c).GetFailed() == nil {
			t.Fatalf("swap = %s, want failed", swapOf(c).MarshalProtoText())
		}
	})
	t.Run("release root admitted on root host", func(t *testing.T) {
		c, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		args, root := candidate(3, true, "peer")
		trusted.Add(bldr_manifest_trust.Root{PluginID: "colors", ManifestRoot: root})
		if n := admitted(instance, args); n != 1 {
			t.Fatalf("release root passed %d candidates, want 1", n)
		}

		// An admitted root never reads as awaiting approval.
		if ok, err := instance.awaitAdmission(ctx, args); err != nil || !ok {
			t.Fatalf("admitted root: admitted %v, err %v", ok, err)
		}
		if swapOf(c).GetCheckingApproval().GetCandidateManifestRev() != args.rev {
			t.Fatalf("swap = %s, want checking approval of the admitted root", swapOf(c).MarshalProtoText())
		}
	})
	t.Run("unapproved root refused on root host", func(t *testing.T) {
		c, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		args, _ := candidate(4, true, "peer")
		awaits(c, instance, args, "peer")
	})
	t.Run("unapproved root refused on space host", func(t *testing.T) {
		// The Space host asks for its own peer, whatever peer the load
		// specifies.
		c, instance := newInstance(&Config{RequireAdmission: true, PeerId: "space-peer"})
		args, _ := candidate(5, true, "other")
		awaits(c, instance, args, "space-peer")
	})
	t.Run("unapproved root refused on host with no owner", func(t *testing.T) {
		// Nothing can admit the root later, so the load fails at once,
		// reporting why, instead of waiting or checking.
		c, instance := newInstance(&Config{PeerId: "root"})
		args, _ := candidate(10, true, "peer")
		if n := admitted(instance, args); n != 0 {
			t.Fatalf("unadmitted root passed %d candidates, want 0", n)
		}
		failed := swapOf(c).GetFailed()
		if failed.GetCandidateManifestRev() != args.rev || !strings.Contains(failed.GetErrorMessage(), errNoApprovalOwner.Error()) {
			t.Fatalf("swap = %s, want failed with no approval owner", swapOf(c).MarshalProtoText())
		}

		// A root whose admission is withdrawn after admittedCandidates passed
		// it meets the same refusal when its start checks again: failed, with
		// no error, so the execute routine does not retry it.
		withdrawn, _ := candidate(11, true, "peer")
		instance.selectedManifest.Store(withdrawn.installation)
		if err := instance.execSelectedCandidate(ctx, withdrawn, false); err != nil {
			t.Fatalf("withdrawn root returned %v, want nil so it is not retried", err)
		}
		failed = swapOf(c).GetFailed()
		if failed.GetCandidateManifestRev() != withdrawn.rev || !strings.Contains(failed.GetErrorMessage(), errNoApprovalOwner.Error()) {
			t.Fatalf("swap = %s, want the withdrawn root failed with no approval owner", swapOf(c).MarshalProtoText())
		}
	})
	t.Run("each fallback is decided by its own peer", func(t *testing.T) {
		// A catalog fallback runs while the preferred load's root waits.
		_, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		preferred, _ := candidate(6, true, "peer")
		catalog, _ := candidate(7, false, "")
		got, err := instance.admittedCandidates(ctx, []*executePluginArgs{preferred, catalog})
		if err != nil || len(got) != 1 || got[0] != catalog {
			t.Fatalf("admitted %d candidates, err %v, want only the catalog fallback", len(got), err)
		}

		// A preferred root with no peer does not refuse an admitted fallback.
		noPeer, _ := candidate(8, true, "")
		fallback, root := candidate(9, true, "peer")
		trusted.Add(bldr_manifest_trust.Root{PluginID: "colors", ManifestRoot: root})
		got, err = instance.admittedCandidates(ctx, []*executePluginArgs{noPeer, fallback})
		if err != nil || len(got) != 1 || got[0] != fallback {
			t.Fatalf("admitted %d candidates, err %v, want only the admitted fallback", len(got), err)
		}
	})
	// resolveFor adds a controller answering AdmitPluginRoot for peerID with
	// fn's resolver.
	resolveFor := func(t *testing.T, peerID string, fn func(context.Context, directive.ResolverHandler) error) {
		t.Helper()
		ctrl := callback.NewCallbackController(
			controller.NewInfo("test/approvals-"+peerID, controller.MustParseVersion("0.0.1"), ""),
			nil,
			func(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
				d, ok := inst.GetDirective().(plugin.AdmitPluginRoot)
				if !ok || d.AdmitPluginRootPeerID() != peerID {
					return nil, nil
				}
				return directive.R(directive.NewFuncResolver(fn), nil)
			},
			nil,
		)
		release, err := tb.Bus.AddController(ctx, ctrl, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
	}
	t.Run("failed read skipped beside an admitted root", func(t *testing.T) {
		resolveFor(t, "broken", func(context.Context, directive.ResolverHandler) error {
			return errors.New("approval store down")
		})

		// The admitted fallback runs although the preferred root's read failed.
		c, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		broken, _ := candidate(11, true, "broken")
		fallback, root := candidate(12, true, "peer")
		trusted.Add(bldr_manifest_trust.Root{PluginID: "colors", ManifestRoot: root})
		got, err := instance.admittedCandidates(ctx, []*executePluginArgs{broken, fallback})
		if err != nil || len(got) != 1 || got[0] != fallback {
			t.Fatalf("admitted %d candidates, err %v, want only the admitted fallback", len(got), err)
		}

		// Alone, the failed read fails the load and reports its error.
		got, err = instance.admittedCandidates(ctx, []*executePluginArgs{broken})
		if err == nil || len(got) != 0 {
			t.Fatalf("failed read returned %d candidates, err %v", len(got), err)
		}
		failed := swapOf(c).GetFailed()
		if failed == nil || failed.GetCandidateManifestRev() != broken.rev || !strings.Contains(failed.GetErrorMessage(), "approval store down") {
			t.Fatalf("swap = %s, want failed with the read's error", swapOf(c).MarshalProtoText())
		}
	})
	t.Run("unanswered read shows the root it awaits", func(t *testing.T) {
		// A resolver that never goes idle holds the read open.
		resolveFor(t, "silent", func(ctx context.Context, _ directive.ResolverHandler) error {
			<-ctx.Done()
			return ctx.Err()
		})
		c, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		args, _ := candidate(13, true, "silent")
		readCtx, stop := context.WithCancel(ctx)
		defer stop()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = instance.admittedCandidates(readCtx, []*executePluginArgs{args})
		}()

		// While the read hangs, the swap says whose approval of which root is
		// being checked, not that the root awaits approval.
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, err := c.pluginStatus.WaitValueWithValidator(waitCtx, func(snap *plugin.PluginStatusSnapshot) (bool, error) {
			for _, st := range snap.GetExecutions() {
				w := st.GetSwap().GetCheckingApproval()
				return w.GetCandidateManifestRev() == args.rev && w.GetPeerId() == "silent", nil
			}
			return false, nil
		}, nil)
		if err != nil {
			t.Fatalf("swap = %s, want checking approval for silent: %v", swapOf(c).MarshalProtoText(), err)
		}
		select {
		case <-done:
			t.Fatal("read returned before any resolver answered")
		default:
		}
		stop()
		<-done
	})
	t.Run("failed wait fails the load", func(t *testing.T) {
		// The first ask finds no answer. Every later ask fails to read.
		var asks atomic.Int32
		failing := callback.NewCallbackController(
			controller.NewInfo("test/failing-approvals", controller.MustParseVersion("0.0.1"), ""),
			nil,
			func(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
				d, ok := inst.GetDirective().(plugin.AdmitPluginRoot)
				if !ok || d.AdmitPluginRootPeerID() != "failing" || asks.Add(1) == 1 {
					return nil, nil
				}
				return directive.R(directive.NewFuncResolver(func(context.Context, directive.ResolverHandler) error {
					return errors.New("approval store down")
				}), nil)
			},
			nil,
		)
		release, err := tb.Bus.AddController(ctx, failing, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer release()

		// The wait returns the failure and publishes it as a failed swap.
		c, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		args, _ := candidate(10, true, "failing")
		got, err := instance.admittedCandidates(ctx, []*executePluginArgs{args})
		if err == nil || len(got) != 0 {
			t.Fatalf("failed wait returned %d candidates, err %v", len(got), err)
		}
		failed := swapOf(c).GetFailed()
		if failed == nil || failed.GetCandidateManifestRev() != args.rev || !strings.Contains(failed.GetErrorMessage(), "approval store down") {
			t.Fatalf("swap = %s, want failed with the wait's error", swapOf(c).MarshalProtoText())
		}
	})
	// settledSwap publishes a settled swap from a live pass and returns it.
	settledSwap := func(c *Controller, instance *pluginInstance) *plugin.PluginSwap {
		t.Helper()
		instance.setSwapSettled(ctx, time.Now())
		swap := swapOf(c)
		if swap.GetSettled() == nil {
			t.Fatalf("swap = %s, want settled", swap.MarshalProtoText())
		}
		return swap
	}
	t.Run("canceled pass leaves the settled swap", func(t *testing.T) {
		// A pass whose ctx already ended asks about nothing and writes nothing.
		c, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		settled := settledSwap(c, instance)
		first, _ := candidate(15, true, "peer")
		second, _ := candidate(16, true, "peer")
		passCtx, stop := context.WithCancel(ctx)
		stop()
		got, err := instance.admittedCandidates(passCtx, []*executePluginArgs{first, second})
		if err == nil || len(got) != 0 {
			t.Fatalf("canceled pass returned %d candidates, err %v", len(got), err)
		}
		if !swapOf(c).EqualVT(settled) {
			t.Fatalf("swap = %s, want the settled state untouched", swapOf(c).MarshalProtoText())
		}
	})
	t.Run("superseded pass cannot overwrite the newer state", func(t *testing.T) {
		// The old pass hangs reading its first root, with a second to visit.
		resolveFor(t, "stalled", func(ctx context.Context, _ directive.ResolverHandler) error {
			<-ctx.Done()
			return ctx.Err()
		})
		c, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		first, _ := candidate(17, true, "stalled")
		second, _ := candidate(18, true, "peer")
		oldCtx, stopOld := context.WithCancel(ctx)
		defer stopOld()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = instance.admittedCandidates(oldCtx, []*executePluginArgs{first, second})
		}()
		waitCtx, cancelWait := context.WithTimeout(ctx, 5*time.Second)
		defer cancelWait()
		if _, err := c.pluginStatus.WaitValueWithValidator(waitCtx, func(snap *plugin.PluginStatusSnapshot) (bool, error) {
			for _, st := range snap.GetExecutions() {
				return st.GetSwap().GetCheckingApproval().GetCandidateManifestRev() == first.rev, nil
			}
			return false, nil
		}, nil); err != nil {
			t.Fatalf("old pass never checked its first root: %v", err)
		}

		// The routine cancels the old pass before the next starts. The
		// newer pass settles, then the old one finishes behind it.
		stopOld()
		settled := settledSwap(c, instance)
		<-done
		if !swapOf(c).EqualVT(settled) {
			t.Fatalf("swap = %s, want the newer pass's settled state", swapOf(c).MarshalProtoText())
		}
	})
	t.Run("kept worker still starting shows preparing", func(t *testing.T) {
		// The admitted worker runs the selected args but has not finished
		// startup, so selecting it again waits for that startup.
		c, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		settledSwap(c, instance)
		args, _ := candidate(19, false, "")
		worker := &pluginInstance{pluginLoadStateCtr: ccontainer.NewCContainer(
			plugin.NewPluginLoadState(nil, plugin.InitialCapabilityRegistrationPending),
		)}
		instance.pluginUpdateMtx.Lock()
		instance.activeArgs, instance.activeExecution = args, worker
		instance.pluginUpdateMtx.Unlock()
		passCtx, stop := context.WithCancel(ctx)
		defer stop()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = instance.execSelectedCandidate(passCtx, args, false)
		}()

		// The swap leaves the earlier settled state for preparing this root.
		waitCtx, cancelWait := context.WithTimeout(ctx, 5*time.Second)
		defer cancelWait()
		if _, err := c.pluginStatus.WaitValueWithValidator(waitCtx, func(snap *plugin.PluginStatusSnapshot) (bool, error) {
			for _, st := range snap.GetExecutions() {
				return st.GetSwap().GetPreparing().GetCandidateManifestRev() == args.rev, nil
			}
			return false, nil
		}, nil); err != nil {
			t.Fatalf("swap = %s, want preparing the kept worker's root", swapOf(c).MarshalProtoText())
		}
		stop()
		<-done
	})
	// released selects args from the catalog, publishes a swap with stale,
	// releases the catalog selection, and asserts the swap is gone.
	released := func(t *testing.T, rev uint64, worker bool, stale func(*pluginInstance, *executePluginArgs)) {
		t.Helper()
		c, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		args, _ := candidate(rev, false, "")
		instance.pluginUpdateMtx.Lock()
		instance.executePluginRoutine.SetState(args)
		if worker {
			instance.activeArgs = args
			instance.activeExecution = &pluginInstance{pluginLoadStateCtr: ccontainer.NewCContainer(
				plugin.NewPluginLoadState(nil, plugin.InitialCapabilityRegistrationPending),
			)}
		}
		instance.pluginUpdateMtx.Unlock()
		stale(instance, args)
		if swapOf(c) == nil {
			t.Fatal("stale swap was not published")
		}
		instance.releaseCatalogExecution()
		if swap := swapOf(c); swap != nil {
			t.Fatalf("swap = %s after release, want none", swap.MarshalProtoText())
		}
	}
	t.Run("release clears awaiting approval", func(t *testing.T) {
		released(t, 20, false, func(instance *pluginInstance, args *executePluginArgs) {
			instance.setSwapAwaitingApproval(ctx, newSwapCandidate(args), "peer")
		})
	})
	t.Run("release clears failed", func(t *testing.T) {
		released(t, 21, false, func(instance *pluginInstance, args *executePluginArgs) {
			instance.setSwapFailed(ctx, newSwapCandidate(args), "prepare plugin: test")
		})
	})
	t.Run("release clears settled once nothing runs", func(t *testing.T) {
		released(t, 22, true, func(instance *pluginInstance, _ *executePluginArgs) {
			instance.setSwapSettled(ctx, time.Now())
		})
	})
	t.Run("installation release clears awaiting approval", func(t *testing.T) {
		// The installation's candidate waits for approval with no worker, so
		// releasing the installation leaves nothing selected.
		c, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		args, _ := candidate(23, true, "peer")
		instance.pluginUpdateMtx.Lock()
		instance.executePluginRoutine.SetState(args)
		instance.pluginUpdateMtx.Unlock()
		instance.setSwapAwaitingApproval(ctx, newSwapCandidate(args), "peer")
		if swapOf(c) == nil {
			t.Fatal("stale swap was not published")
		}
		instance.pluginUpdateMtx.Lock()
		stop := instance.releaseInstallationLocked()
		instance.pluginUpdateMtx.Unlock()
		if stop != nil {
			t.Fatal("release stopped a worker, want none")
		}
		if swap := swapOf(c); swap != nil {
			t.Fatalf("swap = %s after release, want none", swap.MarshalProtoText())
		}
	})
	t.Run("cleared worker keeps the selected pass's swap", func(t *testing.T) {
		// The worker exits while the routine still selects a candidate,
		// so the pass still owns the swap state and the swap state
		// stays.
		c, instance := newInstance(&Config{PeerId: "root", AdmissionPluginId: "owner"})
		args, _ := candidate(24, false, "")
		worker := &pluginInstance{pluginLoadStateCtr: ccontainer.NewCContainer(
			plugin.NewPluginLoadState(nil, plugin.InitialCapabilityRegistrationPending),
		)}
		instance.pluginUpdateMtx.Lock()
		instance.executePluginRoutine.SetState(args)
		instance.activeArgs, instance.activeExecution = args, worker
		instance.pluginUpdateMtx.Unlock()
		instance.setSwapAwaitingApproval(ctx, newSwapCandidate(args), "peer")
		want := swapOf(c)
		if want == nil {
			t.Fatal("swap was not published")
		}
		instance.clearExecution(worker)
		if !swapOf(c).EqualVT(want) {
			t.Fatalf("swap = %s after the worker cleared, want the pass's state kept", swapOf(c).MarshalProtoText())
		}
	})
}

// ownerPlugin is the approvals owner's running plugin, reached through client.
type ownerPlugin struct{ client srpc.Client }

// GetRpcClient returns the owner's client.
func (p ownerPlugin) GetRpcClient() srpc.Client { return p.client }

// TestReleaseHostAsksApprovalsOwner checks that a release host scheduler runs a
// root its trust set does not admit only once the device's approvals, asked
// through the plugin that owns them, admit it for the installation peer.
func TestReleaseHostAsksApprovalsOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	const peerID, ownerID = "session-peer", "spacewave-core"

	// The owner's bus holds the device's approvals, as the root resource
	// controller adds them on the spacewave-core plugin bus.
	core, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Release()
	approvals := plugin_approval.NewApprovals(core.Bus, core.Volume.GetID(), "test-approvals")
	relApprovals, err := core.Bus.AddController(ctx, plugin_approval.NewController(le, approvals), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relApprovals()
	mux := srpc.NewMux()
	if err := bldr_plugin_admit.SRPCRegisterPluginRootAdmission(mux, bldr_plugin_admit.NewViaBus(core.Bus)); err != nil {
		t.Fatal(err)
	}
	owner := ownerPlugin{srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))}

	// newHost builds a release host scheduler with an empty trust set and the
	// admit controller, running the owner when ownerRuns is set.
	newHost := func(ownerRuns bool) (*Controller, *pluginInstance) {
		t.Helper()
		host, err := testbed.NewTestbed(ctx, le)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(host.Release)
		ctrls := []controller.Controller{
			bldr_manifest_trust.NewController(bldr_manifest_trust.NewSet()),
			bldr_plugin_admit.NewController(le, host.Bus, ownerID),
		}
		if ownerRuns {
			ctrls = append(ctrls, callback.NewCallbackController(
				controller.NewInfo("test/owner", controller.MustParseVersion("0.0.1"), ""),
				nil,
				func(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
					d, ok := inst.GetDirective().(plugin.LoadPlugin)
					if !ok || d.LoadPluginID() != ownerID {
						return nil, nil
					}
					return directive.R(directive.NewValueResolver([]plugin.RunningPlugin{owner}), nil)
				},
				nil,
			))
		}
		for _, ctrl := range ctrls {
			rel, err := host.Bus.AddController(ctx, ctrl, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(rel)
		}
		c := &Controller{
			le: le, bus: host.Bus, conf: &Config{PeerId: "host-peer", AdmissionPluginId: ownerID},
		}
		_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
		c.setPluginStatus("colors", "", plugin.PluginState_PluginState_REQUESTED)
		return c, instance
	}
	// candidate is the Space's load of rev for the session peer.
	candidate := func(rev uint64) (*executePluginArgs, string) {
		h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte{0xa1, byte(rev)})
		if err != nil {
			t.Fatal(err)
		}
		ref := newTestManifestRef("colors", "js", rev, "candidate").GetManifestRef()
		ref.RootRef = &block.BlockRef{Hash: h}
		return &executePluginArgs{
			pluginHost:       &testPluginHost{id: "test"},
			rev:              rev,
			manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: ref},
			installation:     &installedManifests{peerID: peerID},
		}, manifestRootKey(ref)
	}
	approve := func(root string) {
		t.Helper()
		if err := approvals.Set(ctx, peerID, &s4wave_plugin_approval.PluginApproval{
			PluginId: "colors", ManifestRoot: root,
			State: s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED,
		}); err != nil {
			t.Fatal(err)
		}
	}
	swapOf := func(c *Controller) *plugin.PluginSwap {
		for _, st := range c.pluginStatus.GetValue().GetExecutions() {
			return st.GetSwap()
		}
		return nil
	}
	// pass runs one admission pass that gives up after a moment.
	pass := func(instance *pluginInstance, args *executePluginArgs) int {
		t.Helper()
		passCtx, stop := context.WithTimeout(ctx, 500*time.Millisecond)
		defer stop()
		got, _ := instance.admittedCandidates(passCtx, []*executePluginArgs{args})
		return len(got)
	}

	t.Run("approved root starts", func(t *testing.T) {
		_, instance := newHost(true)
		args, root := candidate(1)
		approve(root)
		if got := pass(instance, args); got != 1 {
			t.Fatalf("approved root: %d candidates admitted, want 1", got)
		}
		t.Log("started: the approvals owner admitted the root")
	})
	t.Run("approval granted after the ask began admits", func(t *testing.T) {
		c, instance := newHost(true)
		args, root := candidate(2)
		done := make(chan error, 1)
		go func() {
			ok, err := instance.awaitAdmission(ctx, args)
			if err == nil && !ok {
				err = errors.New("refused")
			}
			done <- err
		}()
		waitCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		if _, err := c.pluginStatus.WaitValueWithValidator(waitCtx, func(snap *plugin.PluginStatusSnapshot) (bool, error) {
			for _, st := range snap.GetExecutions() {
				return st.GetSwap().GetAwaitingApproval().GetCandidateManifestRev() == args.rev, nil
			}
			return false, nil
		}, nil); err != nil {
			t.Fatalf("swap = %s, want awaiting approval", swapOf(c).MarshalProtoText())
		}
		approve(root)
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("wait ended with %v, want admitted", err)
			}
		case <-waitCtx.Done():
			t.Fatal("approval granted during the wait never admitted the root")
		}
	})
	t.Run("owner not running shows checking approval", func(t *testing.T) {
		c, instance := newHost(false)
		args, root := candidate(3)
		approve(root)
		if got := pass(instance, args); got != 0 {
			t.Fatalf("owner not running: %d candidates admitted, want 0", got)
		}
		checking := swapOf(c).GetCheckingApproval()
		if checking.GetCandidateManifestRev() != args.rev || checking.GetPeerId() != peerID {
			t.Fatalf("swap = %s, want checking approval for %q", swapOf(c).MarshalProtoText(), peerID)
		}
	})
	t.Run("root neither trusted nor approved awaits approval", func(t *testing.T) {
		c, instance := newHost(true)
		args, _ := candidate(4)
		if got := pass(instance, args); got != 0 {
			t.Fatalf("unapproved root: %d candidates admitted, want 0", got)
		}
		waiting := swapOf(c).GetAwaitingApproval()
		if waiting.GetCandidateManifestRev() != args.rev || waiting.GetPeerId() != peerID {
			t.Fatalf("swap = %s, want awaiting approval for %q", swapOf(c).MarshalProtoText(), peerID)
		}
	})
}
