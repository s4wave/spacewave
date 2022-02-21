package testbed

import (
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	peer_controller "github.com/aperturerobotics/bifrost/peer/controller"
	forge_cluster "github.com/aperturerobotics/forge/cluster"
	cluster_controller "github.com/aperturerobotics/forge/cluster/controller"
	forge_job "github.com/aperturerobotics/forge/job"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_worker "github.com/aperturerobotics/forge/worker"
	forge_world "github.com/aperturerobotics/forge/world"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/identity"
	"github.com/aperturerobotics/timestamp"
	"github.com/pkg/errors"
)

// RunWorkerWithTasks runs a set of Task using the Cluster, Worker, Pass controllers.
func (tb *Testbed) RunWorkerWithTasks(
	taskMap map[string]*forge_target.Target,
	valueSet *forge_target.ValueSet,
	replicas uint32,
	ts *timestamp.Timestamp,
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

	// create a new peer for the cluster
	clusterPeer, err := peer.NewPeer(nil)
	if err != nil {
		return nil, err
	}

	// create the Cluster object in the world
	clusterKey := "cluster/1"
	clusterName := "test-cluster"
	_, _, err = forge_cluster.CreateCluster(
		ctx,
		worldState,
		clusterKey,
		clusterName,
		clusterPeer.GetPeerID(),
		sender,
	)
	if err != nil {
		return nil, err
	}

	// create a new peer for the worker
	workerPeer, err := peer.NewPeer(nil)
	if err != nil {
		return nil, err
	}

	// create keypair for the worker
	workerKeypair, err := identity.NewKeypair(workerPeer.GetPubKey(), "", nil)
	if err != nil {
		return nil, err
	}

	// attach the worker peer controller
	workerPeerCtrl, err := peer_controller.NewController(le, workerPeer.GetPrivKey())
	if err != nil {
		return nil, err
	}
	go func() {
		_ = tb.Bus.ExecuteController(ctx, workerPeerCtrl)
	}()

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

	// assign the Worker to the Cluster
	_, _, err = forge_cluster.AssignWorkerToCluster(ctx, worldState, clusterKey, workerKey, sender)
	if err != nil {
		return nil, err
	}

	// create the Job and Task with empty peer ID
	jobKey := "job/1"
	_, _, err = forge_job.CreateJobWithTasks(
		ctx,
		worldState,
		sender,
		jobKey,
		taskMap,
		"",
		ts,
	)
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
	if len(jobTasks) != 1 {
		return nil, errors.Errorf("expected %d job tasks but found %d", 1, len(jobTasks))
	}
	if len(jobTaskKeys) != len(jobTasks) {
		return nil, errors.Errorf("expected %d job task keys but found %d", len(jobTasks), len(jobTaskKeys))
	}

	// start the Cluster controller, which will schedule the Job to the Worker.
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
