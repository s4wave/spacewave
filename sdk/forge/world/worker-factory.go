package s4wave_forge_world

import (
	"context"
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
	pass_controller "github.com/s4wave/spacewave/forge/pass/controller"
	task_controller "github.com/s4wave/spacewave/forge/task/controller"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// forgeWorkerFactory creates a ForgeWorker resource with PersistentExecutionService.
// Checks that the Worker's linked keypair peer ID matches the local session
// peer ID. Returns nil invoker (no-op) if they don't match, so only the
// creating session runs the worker loop.
func forgeWorkerFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	// Get the local session peer ID from context.
	sessionPeerID := objecttype.SessionPeerIDFromContext(ctx)
	if len(sessionPeerID) == 0 {
		// No session peer ID available; cannot run as worker.
		return nil, func() {}, nil
	}

	// Look up the Worker's linked keypairs to derive its peer ID.
	workerPeerID, err := resolveWorkerPeerID(ctx, ws, objectKey)
	if err != nil {
		return nil, nil, err
	}
	if len(workerPeerID) == 0 {
		// Worker has no linked keypair; cannot determine owner.
		return nil, func() {}, nil
	}

	// Only the session whose peer ID matches runs the worker loop.
	if workerPeerID != sessionPeerID {
		return nil, func() {}, nil
	}

	engineID := objecttype.EngineIDFromContext(ctx)
	resource := &forgeWorkerResource{
		objectKey: objectKey,
		ws:        ws,
		b:         b,
		le:        le,
		peerID:    sessionPeerID,
		engineID:  engineID,
	}
	mux := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_process.SRPCRegisterPersistentExecutionService(mux, resource)
	})
	return mux, func() {}, nil
}

// resolveWorkerPeerID looks up the Worker's linked keypairs and returns the
// peer ID derived from the first keypair. Returns empty if no keypair linked.
func resolveWorkerPeerID(ctx context.Context, ws world.WorldState, objectKey string) (peer.ID, error) {
	kps, _, err := forge_worker.CollectWorkerKeypairs(ctx, ws, objectKey)
	if err != nil {
		return "", err
	}
	for _, kp := range kps {
		if kp == nil {
			continue
		}
		pid, err := kp.ParsePeerID()
		if err != nil {
			continue
		}
		return pid, nil
	}
	return "", nil
}

// forgeWorkerResource implements PersistentExecutionService for a Forge Worker.
type forgeWorkerResource struct {
	objectKey string
	ws        world.WorldState
	b         bus.Bus
	le        *logrus.Entry
	peerID    peer.ID
	engineID  string
}

// Execute implements SRPCPersistentExecutionServiceServer.
// Sends RUNNING status, registers forge controller factories on the bus,
// starts the forge WorkerController which discovers assigned objects and
// starts Cluster/Task/Pass/Execution controllers, then blocks until canceled.
func (r *forgeWorkerResource) Execute(
	req *s4wave_process.ExecuteRequest,
	stream s4wave_process.SRPCPersistentExecutionService_ExecuteStream,
) error {
	ctx := stream.Context()
	le := r.le.WithField("worker", r.objectKey)

	if err := stream.Send(&s4wave_process.ExecuteStatus{
		State: s4wave_process.ExecutionState_ExecutionState_RUNNING,
	}); err != nil {
		return err
	}

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
	// for each (Cluster, Task, Pass, Execution).
	workerConf := worker_controller.NewConfig(r.engineID, r.objectKey, r.peerID, true)
	workerCtrl := worker_controller.NewController(le, r.b, workerConf)
	workerRelease, err := r.b.AddController(ctx, workerCtrl, func(exitErr error) {
		if exitErr != nil && exitErr != context.Canceled {
			le.WithError(exitErr).Warn("forge worker controller exited")
		}
	})
	if err != nil {
		return errors.Wrap(err, "add forge worker controller")
	}
	defer workerRelease()

	// Periodically send heartbeat status on the stream.
	// The process binding controller observes these to detect worker liveness.
	heartbeatInterval := 30 * time.Second
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := stream.Send(&s4wave_process.ExecuteStatus{
				State: s4wave_process.ExecutionState_ExecutionState_RUNNING,
			}); err != nil {
				le.WithError(err).Debug("heartbeat send failed")
				return err
			}
		}
	}
}

// _ is a type assertion
var _ s4wave_process.SRPCPersistentExecutionServiceServer = (*forgeWorkerResource)(nil)
