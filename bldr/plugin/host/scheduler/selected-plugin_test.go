package plugin_host_scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_trust "github.com/s4wave/spacewave/bldr/manifest/trust"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// candidateActivation exposes the ready candidate's admission boundary.
type candidateActivation struct {
	activated chan struct{}
	// answer, when set before the check, holds Check until it yields the
	// call's result, like a worker that is busy or leaves.
	answer chan error
}

// Check declares support for staged replacement, after answer when set.
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
		pluginStatusCtr: ccontainer.NewCContainer(&plugin.PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*plugin.PluginStatus),
	}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
	c.setPluginStatus("colors", "", plugin.PluginState_PluginState_REQUESTED)

	// waitExecution waits for the colors execution status to satisfy ok and
	// logs the swap state it reached.
	waitExecution := func(phase string, ok func(*plugin.PluginExecutionStatus) bool) {
		t.Helper()
		snap, err := c.pluginStatusCtr.WaitValueWithValidator(ctx, func(snap *plugin.PluginStatusSnapshot) (bool, error) {
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

	// A ready candidate is activated once; the retired worker's teardown cannot
	// clear the new RPC alias. Releasing the binding ends the remaining worker.
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
		_, err := c.pluginStatusCtr.WaitValueWithValidator(ctx, func(snap *plugin.PluginStatusSnapshot) (bool, error) {
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

	// A worker that never answers keeps the swap waiting until a restart ends
	// it; the execute routine then starts the candidate in its place.
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
		pluginStatusCtr: ccontainer.NewCContainer(&plugin.PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*plugin.PluginStatus),
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
	snap, err := c.pluginStatusCtr.WaitValueWithValidator(t.Context(), func(snap *plugin.PluginStatusSnapshot) (bool, error) {
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
		pluginStatusCtr: ccontainer.NewCContainer(&plugin.PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*plugin.PluginStatus),
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

	// An unadmitted root runs nothing; admitting it starts the worker.
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
			pluginStatusCtr: ccontainer.NewCContainer(&plugin.PluginStatusSnapshot{}),
			pluginStatus:    make(map[string]*plugin.PluginStatus),
		}
		_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
		c.setPluginStatus("colors", "", plugin.PluginState_PluginState_REQUESTED)
		return c, instance
	}
	// candidate selects rev, named by a load for peerID when load is set.
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
		for _, st := range c.pluginStatusCtr.GetValue().GetExecutions() {
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
		_, instance := newInstance(&Config{PeerId: "root"})
		args, _ := candidate(1, false, "")
		if n := admitted(instance, args); n != 1 {
			t.Fatalf("plain catalog load passed %d candidates, want 1", n)
		}
	})
	t.Run("manifest load with empty peer refused", func(t *testing.T) {
		c, instance := newInstance(&Config{PeerId: "root"})
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
		_, instance := newInstance(&Config{PeerId: "root"})
		args, root := candidate(3, true, "peer")
		trusted.Add(bldr_manifest_trust.Root{PluginID: "colors", ManifestRoot: root})
		if n := admitted(instance, args); n != 1 {
			t.Fatalf("release root passed %d candidates, want 1", n)
		}
	})
	t.Run("unapproved root refused on root host", func(t *testing.T) {
		c, instance := newInstance(&Config{PeerId: "root"})
		args, _ := candidate(4, true, "peer")
		awaits(c, instance, args, "peer")
	})
	t.Run("unapproved root refused on space host", func(t *testing.T) {
		// The Space host asks for its own peer, whatever the load names.
		c, instance := newInstance(&Config{RequireAdmission: true, PeerId: "space-peer"})
		args, _ := candidate(5, true, "other")
		awaits(c, instance, args, "space-peer")
	})
}
