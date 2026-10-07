package flowgraph_run_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	cluster_controller "github.com/s4wave/spacewave/forge/cluster/controller"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_lib_util_presence "github.com/s4wave/spacewave/forge/lib/util/presence"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_json "github.com/s4wave/spacewave/forge/target/json"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/s4wave/spacewave/forge/testbed"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	forge_world "github.com/s4wave/spacewave/forge/world"
	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_run "github.com/s4wave/spacewave/sdk/flowgraph/run"
)

const (
	// graphKey is the Flowgraph each test runs.
	graphKey = "flowgraph/test"
	// runKey is the run each test drives.
	runKey = "flowgraph-run/test"
	// clusterKey is the Cluster running the run's Job.
	clusterKey = "cluster/1"
	// workerKey is the Worker running the Cluster's Tasks.
	workerKey = "worker/1"
)

// env is a Forge testbed with a Cluster, a stoppable Worker, and the
// presence and run controller factories.
type env struct {
	t   *testing.T
	ctx context.Context
	tb  *testbed.Testbed
	// sender authors the test's World operations.
	sender peer.ID
	// workerPeerID is the Cluster and Worker peer.
	workerPeerID peer.ID
	// dir holds the paths the presence Steps check.
	dir string
}

// newEnv starts a testbed whose Cluster controller runs for the whole test.
func newEnv(t *testing.T) *env {
	// Start the testbed.
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	t.Cleanup(cancel)
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Register the presence and run controller factories and the flag file.
	tb.StaticResolver.AddFactory(forge_lib_util_presence.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(flowgraph_run.NewFactory(tb.Bus))
	e := &env{t: t, ctx: ctx, tb: tb, sender: tb.Volume.GetPeerID(), dir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(e.dir, "flag"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// Handle Forge operations.
	opc := world.NewLookupOpController("forge-ops", tb.EngineID, forge_world.LookupWorldOp)
	releaseOps, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseOps)

	// Attach the Worker peer.
	workerPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	e.workerPeerID = workerPeer.GetPeerID()
	keypair, err := identity.NewKeypair(workerPeer.GetPubKey(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	releasePeer, err := tb.Bus.AddController(ctx, peer_controller.NewController(tb.Logger, workerPeer), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releasePeer)

	// Create the Cluster and its Worker.
	ws := tb.WorldState
	if _, _, err := forge_cluster.CreateCluster(ctx, ws, clusterKey, "test-cluster", e.workerPeerID, e.sender); err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_worker.CreateWorker(ctx, ws, workerKey, "test-worker", []*identity.Keypair{keypair}, e.sender); err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_cluster.AssignWorkerToCluster(ctx, ws, clusterKey, workerKey, e.sender); err != nil {
		t.Fatal(err)
	}

	// Run the Cluster controller independently of the Worker.
	_, clusterRef, err := cluster_controller.StartControllerWithConfig(ctx, tb.Bus, cluster_controller.NewConfig(tb.EngineID, clusterKey, e.workerPeerID))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(clusterRef.Release)
	return e
}

// startWorker starts the Worker controller and returns its stop function.
func (e *env) startWorker() func() {
	// Start the Worker and release it at cleanup or on stop.
	conf := worker_controller.NewConfig(e.tb.EngineID, workerKey, e.workerPeerID, true)
	_, ref, err := worker_controller.StartControllerWithConfig(e.ctx, e.tb.Bus, conf)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(ref.Release)
	return ref.Release
}

// startController starts the run controller and returns its stop function.
func (e *env) startController() func() {
	_, ref, err := flowgraph_run.StartControllerWithConfig(e.ctx, e.tb.Bus, flowgraph_run.NewConfig(e.tb.EngineID, runKey))
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(ref.Release)
	return ref.Release
}

// exec runs fn in one committed World transaction.
func (e *env) exec(fn func(ctx context.Context, tx world.WorldState) error) {
	if err := world.ExecTransaction(e.ctx, e.tb.Engine, true, fn); err != nil {
		e.t.Fatal(err)
	}
}

// createGraph creates the test Flowgraph and returns its revision.
func (e *env) createGraph(nodes map[string]*s4wave_flowgraph.FlowgraphNode, connections map[string]*s4wave_flowgraph.FlowgraphConnection) uint64 {
	var revision uint64
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		op := &s4wave_flowgraph.CreateFlowgraphOp{ObjectKey: graphKey, Name: "test"}
		if _, err := op.ApplyWorldOp(ctx, nil, tx, e.sender); err != nil {
			return err
		}
		return e.update(ctx, tx, &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: nodes, SetConnections: connections}, &revision)
	})
	return revision
}

// editGraph applies an owner edit and returns the new revision.
func (e *env) editGraph(request *s4wave_flowgraph.UpdateFlowgraphRequest) uint64 {
	var revision uint64
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		return e.update(ctx, tx, request, &revision)
	})
	return revision
}

// update applies an edit in a transaction and stores the revision.
func (e *env) update(ctx context.Context, tx world.WorldState, request *s4wave_flowgraph.UpdateFlowgraphRequest, revision *uint64) error {
	snapshot, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, graphKey, request)
	*revision = snapshot.GetRevision()
	return err
}

// startRun creates the run of the test Flowgraph.
func (e *env) startRun() {
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		return flowgraph_run.StartRun(ctx, tx, e.sender, runKey, graphKey, clusterKey, nil)
	})
}

// resume resumes the paused run.
func (e *env) resume() {
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		return flowgraph_run.ResumeRun(ctx, tx, runKey)
	})
}

// step builds a Step with input ports whose presence Target sets the first
// output port when present is true and the second otherwise.
func (e *env) step(inputs []string, outputs [2]string, present bool, bound *s4wave_flowgraph.FlowgraphBound) *s4wave_flowgraph.FlowgraphNode {
	// Check a path that exists only when the first output should be chosen.
	path := filepath.Join(e.dir, "flag")
	if !present {
		path = filepath.Join(e.dir, "absent")
	}
	target, err := target_json.ResolveYAML(e.ctx, e.tb.Bus, []byte(`
outputs:
  - name: `+strconv.Quote(outputs[0])+`
    outputType: OutputType_EXEC
    execOutput: `+strconv.Quote(outputs[0])+`
  - name: `+strconv.Quote(outputs[1])+`
    outputType: OutputType_EXEC
    execOutput: `+strconv.Quote(outputs[1])+`
exec:
  controller:
    id: forge/lib/util/presence
    config:
      path: `+strconv.Quote(path)+`
      presentOutput: `+strconv.Quote(outputs[0])+`
      absentOutput: `+strconv.Quote(outputs[1])+`
`))
	if err != nil {
		e.t.Fatal(err)
	}

	// Declare the input ports and both output ports.
	node := &s4wave_flowgraph.FlowgraphNode{
		TypeId: s4wave_flowgraph.StepNodeTypeID,
		Step:   &s4wave_flowgraph.FlowgraphStep{Target: target, Bound: bound},
	}
	for _, name := range inputs {
		node.Ports = append(node.Ports, port(name, s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT))
	}
	for _, name := range outputs {
		node.Ports = append(node.Ports, port(name, s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT))
	}
	return node
}

// port builds an activation port.
func port(name string, direction s4wave_flowgraph.FlowgraphPortDirection) *s4wave_flowgraph.FlowgraphPort {
	return &s4wave_flowgraph.FlowgraphPort{Name: name, Direction: direction, TypeId: "activation"}
}

// connect builds a connection from an output port to an input port.
func connect(outputNode, outputPort, inputNode, inputPort string) *s4wave_flowgraph.FlowgraphConnection {
	return &s4wave_flowgraph.FlowgraphConnection{
		OutputNode: outputNode,
		OutputPort: outputPort,
		InputNode:  inputNode,
		InputPort:  inputPort,
	}
}

// readRun reads the run state, or nil before it exists.
func (e *env) readRun() *s4wave_flowgraph.FlowgraphRun {
	// Look up the run object.
	obj, found, err := e.tb.WorldState.GetObject(e.ctx, runKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		e.t.Fatal(err)
	}
	if !found {
		return nil
	}

	// Decode its state.
	var run *s4wave_flowgraph.FlowgraphRun
	_, _, err = world.AccessObjectState(e.ctx, obj, false, func(cursor *block.Cursor) error {
		var err error
		run, err = s4wave_flowgraph.UnmarshalFlowgraphRun(e.ctx, cursor)
		return err
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return run
}

// readJob reads the run's Job.
func (e *env) readJob() *forge_job.Job {
	job, err := forge_job.LookupJobBody(e.ctx, e.tb.WorldState, flowgraph_run.JobKey(runKey))
	if err != nil {
		e.t.Fatal(err)
	}
	return job
}

// waitFor waits on World changes until done holds.
func (e *env) waitFor(what string, done func() bool) {
	e.t.Helper()
	for {
		seqno, err := e.tb.WorldState.GetSeqno(e.ctx)
		if err != nil {
			e.t.Fatal(err)
		}
		if done() {
			return
		}
		if _, err := e.tb.WorldState.WaitSeqno(e.ctx, seqno+1); err != nil {
			e.t.Fatalf("waiting for %s: %v", what, err)
		}
	}
}

// waitRun waits until the run reaches a state and returns it.
func (e *env) waitRun(state s4wave_flowgraph.FlowgraphRunState) *s4wave_flowgraph.FlowgraphRun {
	e.t.Helper()
	var run *s4wave_flowgraph.FlowgraphRun
	e.waitFor("run "+state.String(), func() bool {
		run = e.readRun()
		return run.GetState() == state && !hasRunning(run)
	})
	return run
}

// waitJob waits until the run's Job reaches a state.
func (e *env) waitJob(state forge_job.State) {
	e.t.Helper()
	e.waitFor("job "+state.String(), func() bool {
		return e.readJob().GetJobState() == state
	})
}

// hasRunning reports whether a run has an activation not yet routed.
func hasRunning(run *s4wave_flowgraph.FlowgraphRun) bool {
	for _, activation := range run.GetActivations() {
		if !activation.GetDone() {
			return true
		}
	}
	return false
}

// trace returns the activated nodes and their chosen outputs in start order.
func trace(run *s4wave_flowgraph.FlowgraphRun) []string {
	var steps []string
	for _, activation := range run.GetActivations() {
		steps = append(steps, activation.GetNodeId()+"."+activation.GetOutput())
	}
	return steps
}

// requireTrace fails unless the run activated exactly the expected steps.
func requireTrace(t *testing.T, run *s4wave_flowgraph.FlowgraphRun, want ...string) {
	t.Helper()
	got := trace(run)
	if len(got) != len(want) {
		t.Fatalf("trace = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("trace = %v, want %v", got, want)
		}
	}
}

// requireInput fails unless an activation's Task reads an input port from
// another activation's chosen output.
func (e *env) requireInput(run *s4wave_flowgraph.FlowgraphRun, activation int, input string, source int) {
	// Read the activation's Task Target.
	e.t.Helper()
	target, _, err := forge_task.LookupTaskTarget(e.ctx, e.tb.WorldState, run.GetActivations()[activation].GetTaskKey())
	if err != nil {
		e.t.Fatal(err)
	}

	// Find the input and compare its Task output reference.
	from := run.GetActivations()[source]
	for _, in := range target.GetInputs() {
		if in.GetName() != input {
			continue
		}
		if in.GetInputType() != forge_target.InputType_InputType_TASK_OUTPUT ||
			in.GetTaskOutput().GetTaskKey() != from.GetTaskKey() ||
			in.GetTaskOutput().GetOutputName() != from.GetOutput() {
			e.t.Fatalf("input %s = %v, want output %s of %s", input, in, from.GetOutput(), from.GetTaskKey())
		}
		return
	}
	e.t.Fatalf("activation %d has no input %s", activation, input)
}

// TestBranch follows the one output port a Step sets.
func TestBranch(t *testing.T) {
	// Start a run of a Step whose two outputs lead to different Steps.
	e := newEnv(t)
	e.createGraph(map[string]*s4wave_flowgraph.FlowgraphNode{
		"a":   e.step(nil, [2]string{"yes", "no"}, true, nil),
		"on":  e.step([]string{"in"}, [2]string{"done", "other"}, true, nil),
		"off": e.step([]string{"in"}, [2]string{"done", "other"}, true, nil),
	}, map[string]*s4wave_flowgraph.FlowgraphConnection{
		"yes": connect("a", "yes", "on", "in"),
		"no":  connect("a", "no", "off", "in"),
	})
	e.startWorker()
	e.startRun()
	e.startController()

	// Only the set output's branch runs, reading that output.
	run := e.waitRun(s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_COMPLETE)
	requireTrace(t, run, "a.yes", "on.done")
	e.requireInput(run, 1, "in", 0)
	e.waitJob(forge_job.State_JobState_COMPLETE)
}

// TestNoOutputFailsStep pauses at a Step that sets no output port and keeps
// its input for a resume.
func TestNoOutputFailsStep(t *testing.T) {
	// Rename b's chosen port, so its Target sets an output that is no port.
	e := newEnv(t)
	b := e.step([]string{"in"}, [2]string{"done", "spare"}, true, nil)
	b.Ports[1].Name = "finished"
	e.createGraph(map[string]*s4wave_flowgraph.FlowgraphNode{
		"a": e.step(nil, [2]string{"yes", "no"}, true, nil),
		"b": b,
	}, map[string]*s4wave_flowgraph.FlowgraphConnection{
		"yes": connect("a", "yes", "b", "in"),
	})
	e.startWorker()
	e.startRun()
	e.startController()

	// The run pauses at b with b's input still queued.
	run := e.waitRun(s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_PAUSED)
	requireTrace(t, run, "a.yes", "b.")
	if run.GetPausedNode() != "b" || run.GetPauseReason() != "step set 0 output ports, want one" {
		t.Fatalf("paused at %q: %q", run.GetPausedNode(), run.GetPauseReason())
	}
	if arrivals := run.GetArrivals(); len(arrivals) != 1 || arrivals[0].GetNodeId() != "b" {
		t.Fatalf("arrivals = %v, want b's input", arrivals)
	}
}

// loopGraph is a source Step feeding a Step that loops to itself while its
// flag exists, bounded to two visits.
func (e *env) loopGraph() uint64 {
	return e.createGraph(map[string]*s4wave_flowgraph.FlowgraphNode{
		"loop":  e.step([]string{"in"}, [2]string{"again", "exit"}, true, &s4wave_flowgraph.FlowgraphBound{MaxVisits: 2}),
		"start": e.step(nil, [2]string{"go", "stop"}, true, nil),
		"end":   e.step([]string{"in"}, [2]string{"done", "other"}, true, nil),
	}, map[string]*s4wave_flowgraph.FlowgraphConnection{
		"again": connect("loop", "again", "loop", "in"),
		"exit":  connect("loop", "exit", "end", "in"),
		"go":    connect("start", "go", "loop", "in"),
	})
}

// requirePausedAtLoop checks a run paused at the loop's visit bound while its
// Job reads COMPLETE.
func (e *env) requirePausedAtLoop(run *s4wave_flowgraph.FlowgraphRun) {
	e.t.Helper()
	if run.GetPausedNode() != "loop" || run.GetPauseReason() != "step reached its bound of 2 visits" {
		e.t.Fatalf("paused at %q: %q", run.GetPausedNode(), run.GetPauseReason())
	}
	if count := run.GetVisits()["loop"].GetCount(); count != 2 {
		e.t.Fatalf("loop visits = %d, want 2", count)
	}
	e.waitJob(forge_job.State_JobState_COMPLETE)
}

// TestLoopPauseResume pauses a loop at its bound and resumes it on the
// unedited revision for another bounded round.
func TestLoopPauseResume(t *testing.T) {
	// Start a run of the bounded loop.
	e := newEnv(t)
	revision := e.loopGraph()
	e.startWorker()
	e.startRun()
	e.startController()

	// The run pauses at the bound while the Job has nothing running.
	run := e.waitRun(s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_PAUSED)
	requireTrace(t, run, "start.go", "loop.again", "loop.again")
	e.requirePausedAtLoop(run)
	e.requireInput(run, 2, "in", 1)

	// Resume continues the loop on the pinned revision until the bound again.
	e.resume()
	run = e.waitRun(s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_PAUSED)
	requireTrace(t, run, "start.go", "loop.again", "loop.again", "loop.again", "loop.again")
	e.requirePausedAtLoop(run)
	if run.GetFlowgraphRevision() != revision {
		t.Fatalf("revision = %d, want %d", run.GetFlowgraphRevision(), revision)
	}
}

// TestResumeEditedRevision resumes a paused loop on the owner's edit, which
// makes the loop exit.
func TestResumeEditedRevision(t *testing.T) {
	// Run the bounded loop until it pauses.
	e := newEnv(t)
	revision := e.loopGraph()
	e.startWorker()
	e.startRun()
	e.startController()
	e.requirePausedAtLoop(e.waitRun(s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_PAUSED))

	// Edit the loop to take its exit, then resume on the newer revision.
	edited := e.editGraph(&s4wave_flowgraph.UpdateFlowgraphRequest{
		SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{
			"loop": e.step([]string{"in"}, [2]string{"again", "exit"}, false, &s4wave_flowgraph.FlowgraphBound{MaxVisits: 2}),
		},
	})
	if edited <= revision {
		t.Fatalf("edited revision %d is not newer than %d", edited, revision)
	}
	e.resume()

	// The paused Step reruns on the edit and the loop exits.
	run := e.waitRun(s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_COMPLETE)
	requireTrace(t, run, "start.go", "loop.again", "loop.again", "loop.exit", "end.done")
	if run.GetFlowgraphRevision() != edited {
		t.Fatalf("revision = %d, want %d", run.GetFlowgraphRevision(), edited)
	}
}

// TestForkJoin forks one output to two Steps and joins both branches.
func TestForkJoin(t *testing.T) {
	// Start a run of a fork into two branches that meet at a join.
	e := newEnv(t)
	e.createGraph(map[string]*s4wave_flowgraph.FlowgraphNode{
		"fork":  e.step(nil, [2]string{"out", "none"}, true, nil),
		"left":  e.step([]string{"in"}, [2]string{"out", "none"}, true, nil),
		"right": e.step([]string{"in"}, [2]string{"out", "none"}, true, nil),
		"join":  e.step([]string{"left", "right"}, [2]string{"out", "none"}, true, nil),
	}, map[string]*s4wave_flowgraph.FlowgraphConnection{
		"fork-left":  connect("fork", "out", "left", "in"),
		"fork-right": connect("fork", "out", "right", "in"),
		"left-join":  connect("left", "out", "join", "left"),
		"right-join": connect("right", "out", "join", "right"),
	})
	e.startWorker()
	e.startRun()
	e.startController()

	// Both branches start from the one output, and the join runs once.
	run := e.waitRun(s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_COMPLETE)
	requireTrace(t, run, "fork.out", "left.out", "right.out", "join.out")
	e.requireInput(run, 1, "in", 0)
	e.requireInput(run, 2, "in", 0)
	e.requireInput(run, 3, "left", 1)
	e.requireInput(run, 3, "right", 2)
}

// TestJobBetweenActivations keeps the run active while the Job tracker marks
// the Job COMPLETE between two activations, and the next activation moves the
// Job back to RUNNING. Stopping the Worker and the run controller holds each
// state long enough to observe it.
func TestJobBetweenActivations(t *testing.T) {
	// Create a run of two Steps in sequence.
	e := newEnv(t)
	e.createGraph(map[string]*s4wave_flowgraph.FlowgraphNode{
		"a": e.step(nil, [2]string{"out", "none"}, true, nil),
		"b": e.step([]string{"in"}, [2]string{"out", "none"}, true, nil),
	}, map[string]*s4wave_flowgraph.FlowgraphConnection{
		"ab": connect("a", "out", "b", "in"),
	})
	e.startRun()

	// The first activation starts the Job.
	stopController := e.startController()
	e.waitFor("first activation", func() bool { return len(e.readRun().GetActivations()) == 1 })
	e.waitJob(forge_job.State_JobState_RUNNING)
	stopController()

	// The Job completes with its only Task while the run stays active.
	stopWorker := e.startWorker()
	e.waitJob(forge_job.State_JobState_COMPLETE)
	stopWorker()
	if run := e.readRun(); run.GetState() != s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING || len(run.GetActivations()) != 1 {
		t.Fatalf("run = %s with %d activations while the Job is COMPLETE", run.GetState(), len(run.GetActivations()))
	}

	// The next activation moves the Job back to RUNNING.
	e.startController()
	e.waitFor("second activation", func() bool { return len(e.readRun().GetActivations()) == 2 })
	e.waitJob(forge_job.State_JobState_RUNNING)

	// The run completes once the Worker runs the second Task.
	e.startWorker()
	run := e.waitRun(s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_COMPLETE)
	requireTrace(t, run, "a.out", "b.out")
	e.waitJob(forge_job.State_JobState_COMPLETE)
}
