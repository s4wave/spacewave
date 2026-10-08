//go:build !tinygo

package s4wave_forge_world

import (
	"context"
	stderrors "errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	forge_lib_docker "github.com/s4wave/spacewave/forge/lib/docker"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	forge_testbed "github.com/s4wave/spacewave/forge/testbed"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	forge_world "github.com/s4wave/spacewave/forge/world"
	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	"github.com/sirupsen/logrus"
)

// TestWorkerHasPeerID accepts every linked session, not just the first keypair.
func TestWorkerHasPeerID(t *testing.T) {
	// Open the Forge testbed for Worker peer relationships.
	ctx := t.Context()
	tb, err := forge_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Attach the World operation controller for creating the Worker.
	op := world.NewLookupOpController("forge-ops", tb.EngineID, forge_world.LookupWorldOp)
	release, err := tb.Bus.AddController(ctx, op, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// Resolve the first linked peer public key.
	firstID := tb.Volume.GetPeerID()
	firstPublic, err := firstID.ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}

	// Create the additional linked and unlinked peers.
	second, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	third, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Build the keypairs for the linked Worker peers.
	firstKeypair, err := identity.NewKeypair(firstPublic, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	secondKeypair, err := identity.NewKeypair(second.GetPubKey(), "", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a Worker linked to both keypairs.
	const workerKey = "workers/multi-peer"
	if _, _, err := forge_worker.CreateWorker(ctx, tb.WorldState, workerKey, "multi-peer",
		[]*identity.Keypair{firstKeypair, secondKeypair}, firstID); err != nil {
		t.Fatal(err)
	}

	// Verify each linked peer can serve the Worker.
	for _, id := range []peer.ID{firstID, second.GetPeerID()} {
		linked, err := workerHasPeerID(ctx, tb.WorldState, workerKey, id)
		if err != nil || !linked {
			t.Fatalf("linked peer %s: linked=%t error=%v", id, linked, err)
		}
	}

	// Verify the unlinked peer cannot serve the Worker.
	linked, err := workerHasPeerID(ctx, tb.WorldState, workerKey, third.GetPeerID())
	if err != nil || linked {
		t.Fatalf("unlinked peer: linked=%t error=%v", linked, err)
	}
}

func TestForgeWorkerExecuteReturnsWorkerControllerError(t *testing.T) {
	// Bound the Worker execution error check.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	// Create a Worker resource whose controller returns an error.
	le := logrus.NewEntry(logrus.New())
	workerPeer, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	controllerErr := stderrors.New("worker watch failed")
	baseBus := inmem.NewBus(directive_controller.NewController(ctx, le))
	workerBus := &workerExitBus{Bus: baseBus, exitErr: controllerErr}
	stream := &forgeWorkerExecuteStream{ctx: ctx}
	resource := &forgeWorkerResource{
		objectKey:            "worker/test",
		b:                    workerBus,
		le:                   le,
		peerID:               workerPeer.GetPeerID(),
		admission:            testWorkerRuntime{},
		openDeclarationWatch: openTestWorkerDeclarationWatch,
	}

	// Run the Worker execution until its controller exits.
	err = resource.Execute(nil, stream)

	// Verify execution preserves the controller error and running status.
	if !stderrors.Is(err, controllerErr) {
		t.Fatalf("Execute error = %v, want %v", err, controllerErr)
	}
	if stream.statuses != 1 {
		t.Fatalf("status count = %d, want 1", stream.statuses)
	}
}

// TestForgeWorkerExecuteReturnsCleanControllerExit keeps a completed Worker
// controller from turning a clean terminal result into a restart error.
func TestForgeWorkerExecuteReturnsCleanControllerExit(t *testing.T) {
	// Create a bounded Worker resource with a clean controller exit.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	workerPeer, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}

	// Connect the Worker resource to the clean-exit controller bus.
	baseBus := inmem.NewBus(directive_controller.NewController(ctx, le))
	resource := &forgeWorkerResource{
		objectKey: "worker/test", b: &workerExitBus{Bus: baseBus}, le: le,
		peerID: workerPeer.GetPeerID(), admission: testWorkerRuntime{},
		openDeclarationWatch: openTestWorkerDeclarationWatch,
	}

	// Verify execution accepts the clean controller exit.
	if err := resource.Execute(nil, &forgeWorkerExecuteStream{ctx: ctx}); err != nil {
		t.Fatalf("clean Worker controller exit = %v", err)
	}
}

// TestForgeWorkerDeclarationRemovalReachesAdmission checks that the Worker's
// enrolled initial declaration and its later removal both reach admission.
func TestForgeWorkerDeclarationRemovalReachesAdmission(t *testing.T) {
	// Create a cancellable Worker execution and local peer.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	workerPeer, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}

	// Connect a declaration stream to the observing Worker admission.
	baseBus := inmem.NewBus(directive_controller.NewController(ctx, le))
	workerBus := &workerExitBus{Bus: baseBus, started: make(chan struct{}), released: make(chan struct{})}
	updates := make(chan *s4wave_device.ForgeWorkerDeclaration, 1)
	applied := make(chan *s4wave_device.ForgeWorkerDeclaration, 2)
	stream := &changingWorkerDeclarationStream{ctx: ctx, updates: updates}
	resource := &forgeWorkerResource{
		objectKey: "worker/test", b: workerBus, le: le, peerID: workerPeer.GetPeerID(),
		admission:            &observingWorkerRuntime{applied: applied, pendingOnRemoval: true},
		openDeclarationWatch: func(context.Context, bus.Bus) (workerDeclarationStream, error) { return stream, nil },
	}

	// Start the Worker execution and wait for its controller.
	done := make(chan error, 1)
	go func() { done <- resource.Execute(nil, &forgeWorkerExecuteStream{ctx: ctx}) }()
	select {
	case <-workerBus.started:
	case <-time.After(time.Second):
		t.Fatal("Worker controller did not start")
	}

	// Verify the enrolled initial declaration reaches Worker admission.
	select {
	case got := <-applied:
		if got.GetWorkerObjectKey() != "worker/test" {
			t.Fatalf("initial declaration = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("initial declaration did not reach admission")
	}

	// Remove the declaration and verify admission receives the removal.
	updates <- nil
	select {
	case got := <-applied:
		if got != nil {
			t.Fatalf("removed declaration = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("declaration removal did not reach admission")
	}

	// Cancel the Worker execution and verify its terminal result.
	cancel()
	if err := <-done; !stderrors.Is(err, context.Canceled) {
		t.Fatalf("Worker cancellation = %v", err)
	}
}

// TestForgeWorkerCancellationRenewsThroughStop proves stream cancellation
// keeps the durable owner claim alive while Docker cleanup holds its debit.
func TestForgeWorkerCancellationRenewsThroughStop(t *testing.T) {
	// Reserve and launch a runtime before Worker cancellation.
	admission, stopper, _ := newWorkerAdmissionTestbed(t)
	grant, err := reserveDocker(t, admission, t.Context(), "exec/cancel", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := grant.Launch(t.Context(), func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}

	// Read the initial Worker lease for the controlled clock.
	initial, err := admission.admission.LookupWorkerCapacityAdmission(t.Context(), "worker/a")
	if err != nil {
		t.Fatal(err)
	}

	// Configure the admission clock and blocked runtime stop.
	base := initial.OwnerLeaseExpiresAt.AsTime().Add(-forge_runtime.DefaultOwnerLeaseDuration)
	var now atomic.Int64
	now.Store(base.UnixNano())
	admission.admission.SetTimeNow(func() time.Time { return time.Unix(0, now.Load()).UTC() })
	stopper.entered = make(chan struct{})
	stopper.release = make(chan struct{})

	// Create a cancellable Worker execution and local peer.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	workerPeer, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}

	// Build the Worker controller bus and initial Docker declaration.
	baseBus := inmem.NewBus(directive_controller.NewController(ctx, le))
	workerBus := &workerExitBus{Bus: baseBus, started: make(chan struct{}), released: make(chan struct{})}
	declaration := &s4wave_device.ForgeWorkerDeclaration{
		WorkerObjectKey: "worker/a", MilliCpu: 2000, MemoryBytes: 2 << 30, Backends: []string{"docker"},
	}
	watch := &changingWorkerDeclarationStream{ctx: ctx, initialDeclaration: declaration}
	renewed := make(chan struct{}, 1)
	renewTick := make(chan time.Time)

	// Connect Worker admission to the controlled renewal clock.
	resource := &forgeWorkerResource{
		objectKey: "worker/a", b: workerBus, le: le, peerID: workerPeer.GetPeerID(),
		admission:            &notifyingWorkerRuntime{WorkerAdmission: admission, stopEntered: stopper.entered, renewed: renewed},
		openDeclarationWatch: func(context.Context, bus.Bus) (workerDeclarationStream, error) { return watch, nil },
		renewTick:            renewTick,
	}

	// Start the Worker execution and wait for its controller.
	done := make(chan error, 1)
	go func() { done <- resource.Execute(nil, &forgeWorkerExecuteStream{ctx: ctx}) }()
	select {
	case <-workerBus.started:
	case <-time.After(5 * time.Second):
		t.Fatal("Worker controller did not start")
	}

	// Cancel execution and wait for Docker cleanup to begin.
	cancel()
	select {
	case <-stopper.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Docker stop did not start")
	}

	// Move the controlled clock near expiry and wait for the renewal event
	// while the stopper remains blocked, without waiting for wall time.
	now.Store(base.Add(50 * time.Second).UnixNano())
	select {
	case renewTick <- time.Unix(0, now.Load()):
	case <-time.After(5 * time.Second):
		t.Fatal("renewal clock was not received")
	}
	select {
	case <-renewed:
	case <-time.After(5 * time.Second):
		t.Fatal("owner claim did not renew during Docker stop")
	}

	// Advance beyond the original deadline and read the renewed claim.
	now.Store(base.Add(65 * time.Second).UnixNano())
	capacity, err := admission.admission.LookupWorkerCapacityAdmission(t.Context(), "worker/a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify blocked Docker cleanup retains the live claim and debit.
	if !time.Unix(0, now.Load()).Before(capacity.OwnerLeaseExpiresAt.AsTime()) {
		t.Fatalf("claim expired during Docker stop: %v", capacity.OwnerLeaseExpiresAt.AsTime())
	}
	if capacity.MilliCPUReserved != 500 || capacity.OwnerState != forge_runtime.CapacityOwnerStateDraining {
		t.Fatalf("blocked stop lost custody: %+v", capacity)
	}

	// Confirm Docker stop and wait for Worker cleanup.
	close(stopper.release)
	select {
	case err := <-done:
		if !stderrors.Is(err, context.Canceled) {
			t.Fatalf("Worker cancellation = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Worker cleanup did not finish")
	}

	// Verify clean drain removed the Worker capacity record.
	if _, err := admission.admission.LookupWorkerCapacityAdmission(t.Context(), "worker/a"); !stderrors.Is(err, forge_runtime.ErrWorkerNotObserved) {
		t.Fatalf("clean drain retained capacity: %v", err)
	}
}

// notifyingWorkerRuntime reports renewal after Docker cleanup has begun.
type notifyingWorkerRuntime struct {
	*WorkerAdmission
	stopEntered <-chan struct{}
	renewed     chan<- struct{}
}

// Renew forwards the durable renewal and signals that it ran during stop.
func (r *notifyingWorkerRuntime) Renew(ctx context.Context) error {
	err := r.WorkerAdmission.Renew(ctx)
	select {
	case <-r.stopEntered:
		select {
		case r.renewed <- struct{}{}:
		default:
		}
	default:
	}
	return err
}

// observingWorkerRuntime records the declarations presented to admission.
type observingWorkerRuntime struct {
	// applied receives each delivered declaration.
	applied chan *s4wave_device.ForgeWorkerDeclaration
	// pendingOnRemoval makes removal report retained stop custody.
	pendingOnRemoval bool
}

// Reserve rejects Docker work outside this declaration delivery test.
func (*observingWorkerRuntime) Reserve(context.Context, string, *forge_lib_docker.Config) (forge_lib_docker.Reservation, error) {
	return nil, stderrors.New("unexpected Docker reservation")
}

// ApplyDeclaration records the current Worker declaration or its removal.
func (r *observingWorkerRuntime) ApplyDeclaration(_ context.Context, key string, declaration *s4wave_device.ForgeWorkerDeclaration) error {
	if key != "devices/self" {
		return stderrors.New("Worker lost enrolled Device identity")
	}
	r.applied <- declaration
	if declaration == nil && r.pendingOnRemoval {
		return ErrWorkerStopPending
	}
	return nil
}

// Renew has no lease effect in this declaration delivery test.
func (*observingWorkerRuntime) Renew(context.Context) error { return nil }

// Close has no World state to drain in this declaration delivery test.
func (*observingWorkerRuntime) Close(context.Context) error { return nil }

// changingWorkerDeclarationStream supplies an enrolled initial declaration and
// changes.
type changingWorkerDeclarationStream struct {
	ctx                context.Context
	updates            <-chan *s4wave_device.ForgeWorkerDeclaration
	initial            bool
	initialDeclaration *s4wave_device.ForgeWorkerDeclaration
}

// Recv returns the current declaration before waiting for an update.
func (s *changingWorkerDeclarationStream) Recv() (*s4wave_device.ForgeWorkerDeclaration, string, error) {
	if !s.initial {
		s.initial = true
		if s.initialDeclaration != nil {
			return s.initialDeclaration, "devices/self", nil
		}
		return &s4wave_device.ForgeWorkerDeclaration{WorkerObjectKey: "worker/test"}, "devices/self", nil
	}
	select {
	case <-s.ctx.Done():
		return nil, "", s.ctx.Err()
	case declaration := <-s.updates:
		return declaration, "devices/self", nil
	}
}

// Close leaves cancellation to the owning execution context.
func (*changingWorkerDeclarationStream) Close() {}

type workerExitBus struct {
	bus.Bus
	exitErr  error
	started  chan struct{}
	released chan struct{}
}

func (b *workerExitBus) AddController(
	ctx context.Context,
	ctrl controller.Controller,
	cb func(error),
) (func(), error) {
	if _, ok := ctrl.(*worker_controller.Controller); ok {
		if b.started != nil {
			close(b.started)
			return func() { close(b.released) }, nil
		}
		cb(b.exitErr)
		return func() {}, nil
	}
	return b.Bus.AddController(ctx, ctrl, cb)
}

type forgeWorkerExecuteStream struct {
	srpc.Stream
	ctx      context.Context
	statuses int
}

func (s *forgeWorkerExecuteStream) Context() context.Context {
	return s.ctx
}

func (s *forgeWorkerExecuteStream) Send(*s4wave_process.ExecuteStatus) error {
	s.statuses++
	return nil
}

func (s *forgeWorkerExecuteStream) SendAndClose(status *s4wave_process.ExecuteStatus) error {
	return s.Send(status)
}

var _ s4wave_process.SRPCPersistentExecutionService_ExecuteStream = (*forgeWorkerExecuteStream)(nil)

// TestForgeWorkerSteadyStatusAndCancellation advances virtual time through four
// former heartbeat periods and checks controller release on stream cancellation.
func TestForgeWorkerSteadyStatusAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create a cancellable Worker execution in virtual time.
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		le := logrus.NewEntry(logrus.New())
		workerPeer, _, _, err := peer.NewPeerWithGenerateED25519()
		if err != nil {
			t.Fatal(err)
		}

		// Connect the Worker resource to its lifecycle observation bus.
		base := inmem.NewBus(directive_controller.NewController(ctx, le))
		workerBus := &workerExitBus{Bus: base, started: make(chan struct{}), released: make(chan struct{})}
		stream := &forgeWorkerExecuteStream{ctx: ctx}
		resource := &forgeWorkerResource{objectKey: "worker/test", b: workerBus, le: le, peerID: workerPeer.GetPeerID(), admission: testWorkerRuntime{}, openDeclarationWatch: openTestWorkerDeclarationWatch}

		// Start the Worker execution and wait for its controller.
		done := make(chan error, 1)
		go func() { done <- resource.Execute(nil, stream) }()
		<-workerBus.started

		// Virtual elapsed time proves that steady execution emits no heartbeat.
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if stream.statuses != 1 {
			t.Fatalf("steady worker emitted %d statuses, want one", stream.statuses)
		}

		// Cancel the Worker execution and verify controller release.
		cancel()
		if err := <-done; !stderrors.Is(err, context.Canceled) {
			t.Fatalf("cancelled execution returned %v", err)
		}
		<-workerBus.released
	})
}

// testWorkerRuntime is the offline Worker admission seam for lifecycle tests.
type testWorkerRuntime struct{}

// Reserve is unused by the Worker lifecycle tests.
func (testWorkerRuntime) Reserve(context.Context, string, *forge_lib_docker.Config) (forge_lib_docker.Reservation, error) {
	return nil, stderrors.New("unexpected Docker reservation")
}

// ApplyDeclaration accepts the test stream's initial empty declaration.
func (testWorkerRuntime) ApplyDeclaration(context.Context, string, *s4wave_device.ForgeWorkerDeclaration) error {
	return nil
}

// Renew keeps virtual lease deadlines from changing this test's focus.
func (testWorkerRuntime) Renew(context.Context) error { return nil }

// Close keeps lifecycle tests isolated from World admission.
func (testWorkerRuntime) Close(context.Context) error { return nil }

// testWorkerDeclarationStream emits no declaration then waits for closure.
type testWorkerDeclarationStream struct {
	ctx     context.Context
	cancel  context.CancelFunc
	initial bool
}

// openTestWorkerDeclarationWatch opens an isolated in-memory declaration stream.
func openTestWorkerDeclarationWatch(context.Context, bus.Bus) (workerDeclarationStream, error) {
	ctx, cancel := context.WithCancel(context.Background())
	return &testWorkerDeclarationStream{ctx: ctx, cancel: cancel}, nil
}

// Recv emits one empty declaration and then follows stream cancellation.
func (s *testWorkerDeclarationStream) Recv() (*s4wave_device.ForgeWorkerDeclaration, string, error) {
	if !s.initial {
		s.initial = true
		return nil, "", nil
	}
	<-s.ctx.Done()
	return nil, "", s.ctx.Err()
}

// Close cancels the test stream's pending receive.
func (s *testWorkerDeclarationStream) Close() { s.cancel() }
