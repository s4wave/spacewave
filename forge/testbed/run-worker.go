package testbed

import (
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	forge_world "github.com/s4wave/spacewave/forge/world"
	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
)

// RunWorkerWithTasks runs a set of Task using the Cluster, Worker, Pass controllers.
//
// If workerPeer is nil, generates a new peer.
func (tb *Testbed) RunWorkerWithTasks(
	taskMap map[string]*forge_target.Target,
	valueSet *forge_target.ValueSet,
	replicas uint32,
	ts *timestamp.Timestamp,
	jobKey string,
	clusterKey string,
	workerPeer peer.Peer,
) (*forge_job.Job, error) {
	ctx, le, worldState := tb.Context, tb.Logger, tb.WorldState
	sender := tb.Volume.GetPeerID()

	// add op handlers to bus
	opc := world.NewLookupOpController(
		"forge-ops",
		tb.EngineID,
		forge_world.LookupWorldOp,
	)
	go func() {
		_ = tb.Bus.ExecuteController(ctx, opc)
	}()
	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	// create a new peer for the worker, if necessary.
	if workerPeer == nil {
		var err error
		workerPeer, err = peer.NewPeer(nil)
		if err != nil {
			return nil, err
		}
	}

	// create keypair for the worker
	workerKeypair, err := identity.NewKeypair(workerPeer.GetPubKey(), "", nil)
	if err != nil {
		return nil, err
	}

	// attach the worker peer controller
	workerPeerCtrl := peer_controller.NewController(le, workerPeer)
	workerPeerRel, err := tb.Bus.AddController(ctx, workerPeerCtrl, nil)
	if err != nil {
		return nil, err
	}
	defer workerPeerRel()

	// create the Job and Task with empty peer ID
	createJobTx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	_, _, err = forge_job.CreateJobWithTasks(
		ctx,
		createJobTx,
		sender,
		jobKey,
		taskMap,
		"",
		ts,
	)
	if err == nil {
		err = createJobTx.Commit(ctx)
	}
	if err != nil {
		createJobTx.Discard()
		return nil, err
	}

	// create the Cluster object in the world
	clusterName := "test-cluster"
	_, _, err = forge_cluster.CreateCluster(
		ctx,
		worldState,
		clusterKey,
		clusterName,
		workerPeer.GetPeerID(),
		sender,
	)
	if err != nil {
		return nil, err
	}

	// create the Worker
	workerKey := "worker/1"
	workerName := "test-worker"
	_, _, err = forge_worker.CreateWorker(
		ctx,
		worldState,
		workerKey,
		workerName,
		[]*identity.Keypair{workerKeypair},
		sender,
	)
	if err != nil {
		return nil, err
	}

	// start the Worker controller, which will schedule all matching Keypair objects to controllers.
	workerCtrlCfg := worker_controller.NewConfig(
		tb.EngineID,
		clusterKey,
		workerPeer.GetPeerID(),
		true,
	)
	_, workerCtrlRef, err := worker_controller.StartControllerWithConfig(
		ctx,
		tb.Bus,
		workerCtrlCfg,
	)
	if err != nil {
		return nil, err
	}
	defer workerCtrlRef.Release()

	// assign the Worker to the Cluster
	_, _, err = forge_cluster.AssignWorkerToCluster(ctx, worldState, clusterKey, workerKey, sender)
	if err != nil {
		return nil, err
	}

	// assign the job to the cluster
	_, _, err = forge_cluster.AssignJobToCluster(ctx, worldState, clusterKey, jobKey, sender)
	if err != nil {
		return nil, err
	}

	// lookup the list of Task for the Job
	jobTasks, jobTaskKeys, err := forge_job.CollectJobTasks(ctx, worldState, jobKey)
	if err != nil {
		return nil, err
	}
	if len(jobTasks) != len(taskMap) {
		return nil, errors.Errorf("expected %d job tasks but found %d", len(taskMap), len(jobTasks))
	}
	if len(jobTaskKeys) != len(jobTasks) {
		return nil, errors.Errorf("expected %d job task keys but found %d", len(jobTasks), len(jobTaskKeys))
	}

	// start the Cluster controller, which will schedule the Job to the Worker.
	/*
		clusterCtrlCfg := cluster_controller.NewConfig(
			tb.EngineID,
			clusterKey,
			clusterPeer.GetPeerID(),
		)
		_, clusterCtrlRef, err := cluster_controller.StartControllerWithConfig(
			ctx,
			tb.Bus,
			clusterCtrlCfg,
		)
		if err != nil {
			return nil, err
		}
		defer clusterCtrlRef.Release()
	*/

	// wait for Job to complete
	finalState, err := forge_job.WaitJobComplete(
		ctx,
		le.WithField("control-loop", "run-job-wait-complete"),
		tb.WorldState,
		jobKey,
	)
	if err != nil {
		return nil, err
	}

	res := finalState.GetResult()
	if errStr := res.FailError; len(errStr) != 0 {
		return finalState, errors.New(errStr)
	}
	// success
	return finalState, nil
}
