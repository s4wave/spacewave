//go:build !tinygo

package s4wave_forge_world

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	resolver_ctrl "github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/db/world"
	cluster_controller "github.com/s4wave/spacewave/forge/cluster/controller"
	exec_controller "github.com/s4wave/spacewave/forge/execution/controller"
	forge_lib_docker "github.com/s4wave/spacewave/forge/lib/docker"
	forge_lib_util_presence "github.com/s4wave/spacewave/forge/lib/util/presence"
	pass_controller "github.com/s4wave/spacewave/forge/pass/controller"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	task_controller "github.com/s4wave/spacewave/forge/task/controller"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// forgeWorkerFactory creates a ForgeWorker resource with PersistentExecutionService.
// Checks that the local session peer is linked to the Worker. Returns a nil
// invoker for an unlinked session; each linked session runs its own worker loop.
func forgeWorkerFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	// Require a World state before constructing the Worker resource.
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	// Get the local session peer ID from context.
	sessionPeerID := objecttype.SessionPeerIDFromContext(ctx)
	if len(sessionPeerID) == 0 {
		// No session peer ID available; cannot run as worker.
		return nil, func() {}, nil
	}

	// A Worker may serve several Device peers; this session runs only when linked.
	linked, err := workerHasPeerID(ctx, ws, objectKey, sessionPeerID)
	if err != nil {
		return nil, nil, err
	}
	if !linked {
		return nil, func() {}, nil
	}

	// Construct the linked Worker resource and its execution service.
	engineID := objecttype.EngineIDFromContext(ctx)
	resource := &forgeWorkerResource{
		objectKey: objectKey,
		ws:        ws,
		b:         b,
		le:        le,
		peerID:    sessionPeerID,
		engineID:  engineID,
		admission: NewWorkerAdmission(engine, objectKey, sessionPeerID, rand.Text(), forge_lib_docker.NewStopper(forge_lib_docker.NewExecDockerRunner())),
		openDeclarationWatch: func(ctx context.Context, b bus.Bus) (workerDeclarationStream, error) {
			return openWorkerDeclarationWatch(ctx, b)
		},
	}
	mux := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_process.SRPCRegisterPersistentExecutionService(mux, resource)
	})
	return mux, func() {}, nil
}

// workerHasPeerID reports whether the session peer is linked to this Worker.
func workerHasPeerID(ctx context.Context, ws world.WorldState, objectKey string, sessionPeerID peer.ID) (bool, error) {
	kps, _, err := forge_worker.CollectWorkerKeypairs(ctx, ws, objectKey)
	if err != nil {
		return false, err
	}
	for _, kp := range kps {
		pid, err := kp.ParsePeerID()
		if err != nil {
			return false, err
		}
		if pid == sessionPeerID {
			return true, nil
		}
	}
	return false, nil
}

// forgeWorkerResource implements PersistentExecutionService for a Forge Worker.
type forgeWorkerResource struct {
	// objectKey identifies the persistent worker object.
	objectKey string
	// ws is the borrowed worker world state.
	ws world.WorldState
	// b owns the execution's controllers.
	b bus.Bus
	// le reports worker failures.
	le *logrus.Entry
	// peerID supplies the local worker authority.
	peerID peer.ID
	// engineID identifies the worker's World engine.
	engineID string
	// admission owns this Worker's sole capacity claim and Docker lifecycle.
	admission workerRuntime
	// openDeclarationWatch connects the Worker to the daemon-owned declaration.
	openDeclarationWatch func(context.Context, bus.Bus) (workerDeclarationStream, error)
	// renewTick supplies a controlled renewal clock in tests.
	renewTick <-chan time.Time
}

// workerRuntime is the Worker's capacity and Docker admission contract.
type workerRuntime interface {
	forge_lib_docker.Admission
	// ApplyDeclaration claims the declared capacity, or drains it when the declaration is nil.
	ApplyDeclaration(context.Context, string, *s4wave_device.ForgeWorkerDeclaration) error
	// Renew extends the Worker's owner claim.
	Renew(context.Context) error
	// Close drains the Worker's remaining runtimes on a clean exit.
	Close(context.Context) error
}

// workerDeclarationStream receives current and changed Forge Worker declarations.
type workerDeclarationStream interface {
	// Recv returns the next declaration and its Device key. The declaration is
	// nil while the Device declares no Worker.
	Recv() (*s4wave_device.ForgeWorkerDeclaration, string, error)
	// Close ends the watch.
	Close()
}

// Execute implements SRPCPersistentExecutionServiceServer.
// Sends RUNNING status, registers forge controller factories on the bus,
// starts the forge WorkerController which discovers assigned objects and
// starts Cluster/Task/Pass/Execution controllers, then serves until the stream
// is canceled or the WorkerController exits.
func (r *forgeWorkerResource) Execute(
	req *s4wave_process.ExecuteRequest,
	stream s4wave_process.SRPCPersistentExecutionService_ExecuteStream,
) error {
	// Open the daemon declaration watch for this Worker execution.
	ctx := stream.Context()
	le := r.le.WithField("worker", r.objectKey)
	watch, err := r.openDeclarationWatch(ctx, r.b)
	if err != nil {
		return errors.Wrap(err, "watch forge worker declaration")
	}
	var watchDone <-chan struct{}
	defer func() {
		watch.Close()
		if watchDone != nil {
			<-watchDone
		}
	}()

	// Read the initial declaration and enrolled Device before starting Worker work.
	declaration, deviceKey, err := watch.Recv()
	if err != nil {
		return errors.Wrap(err, "initial forge worker declaration")
	}

	// Keep the owner claim renewing through the bounded Docker stop and
	// durable credit. Stream cancellation only stops new Worker work.
	renewCtx, cancelRenew := context.WithCancel(context.WithoutCancel(ctx))
	renewDone := make(chan struct{})
	renewErr := make(chan error, 1)
	go func() {
		defer close(renewDone)
		renewTick := r.renewTick
		if renewTick == nil {
			renew := time.NewTicker(forge_runtime.DefaultOwnerLeaseDuration / 3)
			defer renew.Stop()
			renewTick = renew.C
		}
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-renewTick:
				if err := r.admission.Renew(renewCtx); err != nil {
					select {
					case renewErr <- err:
					default:
					}
				}
			}
		}
	}()
	defer func() {
		cancelRenew()
		<-renewDone
	}()

	// Apply the initial declaration and retain cleanup custody on exit.
	if err := r.admission.ApplyDeclaration(ctx, deviceKey, declaration); err != nil {
		if !errors.Is(err, ErrWorkerStopPending) {
			return errors.Wrap(err, "observe worker capacity")
		}
		le.WithError(err).Warn("Worker runtime stop pending; retaining capacity claim")
	}
	defer func() {
		// A stop that exceeds this deadline retains its World pending-stop
		// reservation and debit for the next owner to reconcile.
		stopCtx, cancelStop := context.WithTimeout(context.WithoutCancel(ctx), forge_runtime.DefaultOwnerLeaseDuration/3)
		if err := r.admission.Close(stopCtx); err != nil {
			le.WithError(err).Warn("drain Worker capacity on exit")
		}
		cancelStop()
	}()

	// Publish the initial running status for the Worker execution.
	if err := stream.Send(&s4wave_process.ExecuteStatus{
		State: s4wave_process.ExecutionState_ExecutionState_RUNNING,
	}); err != nil {
		return err
	}

	// Attach the local Worker peer controller for this execution.
	workerPeer, err := peer.NewPeerWithID(r.peerID)
	if err != nil {
		return errors.Wrap(err, "build worker peer")
	}
	peerCtrl := peer_controller.NewController(le, workerPeer)
	peerRelease, err := r.b.AddController(ctx, peerCtrl, nil)
	if err != nil {
		return errors.Wrap(err, "add worker peer controller")
	}
	defer peerRelease()

	// Register forge controller factories so the WorkerController can start
	// them via LoadControllerWithConfig directives.
	//
	// Space-aware exec handler bridge factories are included so the execution
	// controller dispatches to SpaceExecRegistry handlers. The plugin bridge
	// receives the bus so it can load plugin-owned exec services.
	execRegistry := space_exec.NewDefaultRegistryWithBus(r.b)
	bridgeFactories := space_exec.BridgeFactories(execRegistry)
	forgeFactories := []controller.Factory{
		cluster_controller.NewFactory(r.b),
		task_controller.NewFactory(r.b),
		pass_controller.NewFactory(r.b),
		exec_controller.NewFactory(r.b),
		forge_lib_docker.NewWorkerFactory(r.b, r.admission),
		forge_lib_util_presence.NewFactory(r.b),
	}
	sr := static.NewResolver(append(forgeFactories, bridgeFactories...)...)
	resolverCtrl := resolver_ctrl.NewController(le, r.b, sr)
	resolverRelease, err := r.b.AddController(ctx, resolverCtrl, nil)
	if err != nil {
		return errors.Wrap(err, "add forge resolver controller")
	}
	defer resolverRelease()

	// Start the forge WorkerController which watches the world for objects
	// assigned to this worker's keypairs and starts the appropriate controller
	// for each (Cluster, Task, Pass, Execution). A controller exit ends this
	// stream so the process lifecycle can restart the Worker.
	workerConf := worker_controller.NewConfig(r.engineID, r.objectKey, r.peerID, true)
	workerCtrl := worker_controller.NewController(le, r.b, workerConf)
	workerExited := make(chan error, 1)
	workerRelease, err := r.b.AddController(ctx, workerCtrl, func(exitErr error) {
		workerExited <- exitErr
	})
	if err != nil {
		return errors.Wrap(err, "add forge worker controller")
	}
	defer workerRelease()

	// Read declaration updates on the stream while the owner lease deadline drives renewal.
	type declarationUpdate struct {
		declaration *s4wave_device.ForgeWorkerDeclaration
		deviceKey   string
		err         error
	}
	updates := make(chan declarationUpdate, 1)
	done := make(chan struct{})
	watchDone = done
	go func() {
		defer close(done)
		for {
			declaration, deviceKey, err := watch.Recv()
			select {
			case updates <- declarationUpdate{declaration: declaration, deviceKey: deviceKey, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case exitErr := <-workerExited:
			if exitErr == nil {
				return nil
			}
			return errors.Wrap(exitErr, "forge worker controller")
		case update := <-updates:
			if update.err != nil {
				return errors.Wrap(update.err, "watch forge worker declaration")
			}
			if err := r.admission.ApplyDeclaration(ctx, update.deviceKey, update.declaration); err != nil {
				if !errors.Is(err, ErrWorkerStopPending) {
					return err
				}
				le.WithError(err).Warn("Worker runtime stop pending; retaining capacity claim")
			}
		case err := <-renewErr:
			if !errors.Is(err, ErrWorkerStopPending) {
				return errors.Wrap(err, "renew worker claim")
			}
			le.WithError(err).Warn("Worker runtime stop pending; retaining capacity claim")
		}
	}
}

// _ is a type assertion.
var _ s4wave_process.SRPCPersistentExecutionServiceServer = (*forgeWorkerResource)(nil)
