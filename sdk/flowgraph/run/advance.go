package flowgraph_run

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/world"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// runner advances one run within a World transaction.
type runner struct {
	// ws is the write transaction.
	ws world.WorldState
	// graph is the pinned Flowgraph body.
	graph *s4wave_flowgraph.Flowgraph
	// run is the run state being advanced.
	run *s4wave_flowgraph.FlowgraphRun
}

// advance routes the completed activations of a run, starts its ready Steps,
// and settles its state. It returns the Task keys of the running activations.
func advance(ctx context.Context, ws world.WorldState, runKey string) ([]string, error) {
	// Read the run and its pinned graph.
	obj, previous, graph, err := readRun(ctx, ws, runKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}

	// Route outcomes, then start Steps and settle the state.
	r := &runner{ws: ws, graph: graph, run: previous.CloneVT()}
	if err := r.route(ctx); err != nil {
		return nil, err
	}
	if err := r.start(ctx); err != nil {
		return nil, err
	}
	r.settle()

	// Write the run only when it changed, so its own revision settles the loop.
	if !r.run.EqualVT(previous) {
		_, _, err = world.AccessObjectState(ctx, obj, true, func(cursor *block.Cursor) error {
			cursor.SetBlock(r.run, true)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return r.running(), nil
}

// readRun reads a run object and its pinned graph. The caller releases obj.
func readRun(ctx context.Context, ws world.WorldState, runKey string) (
	world.ObjectState,
	*s4wave_flowgraph.FlowgraphRun,
	*s4wave_flowgraph.Flowgraph,
	error,
) {
	// Look up the run object.
	obj, found, err := ws.GetObject(ctx, runKey)
	if err != nil {
		return obj, nil, nil, err
	}
	if !found {
		return obj, nil, nil, world.ErrObjectNotFound
	}

	// Decode the run state and follow its graph reference.
	var run *s4wave_flowgraph.FlowgraphRun
	var graph *s4wave_flowgraph.Flowgraph
	_, _, err = world.AccessObjectState(ctx, obj, false, func(cursor *block.Cursor) error {
		// Decode the run; an empty object holds no run.
		var err error
		run, err = s4wave_flowgraph.UnmarshalFlowgraphRun(ctx, cursor)
		if err != nil {
			return err
		}
		if run == nil {
			return errors.New("flowgraph run object is empty")
		}

		// Follow the pinned graph.
		graph, err = run.FollowGraph(ctx, cursor)
		return err
	})
	return obj, run, graph, err
}

// route marks each completed activation done and delivers its chosen output
// along the chosen port's connections. A failed activation requeues its inputs
// and pauses the run at its Step.
func (r *runner) route(ctx context.Context) error {
	for _, activation := range r.run.GetActivations() {
		// Skip routed activations and Tasks that are still running.
		if activation.GetDone() {
			continue
		}
		task, err := forge_task.LookupTaskBody(ctx, r.ws, activation.GetTaskKey())
		if err != nil {
			return err
		}
		if !task.IsComplete() {
			continue
		}

		// Read the outcome; a Step failure leaves the inputs for a resume.
		port, spend, failure, err := r.outcome(ctx, activation.GetNodeId(), task)
		if err != nil {
			return err
		}
		activation.Done = true
		if failure != "" {
			r.run.Arrivals = append(r.run.Arrivals, activation.GetInputs()...)
			r.pause(activation.GetNodeId(), failure)
			continue
		}

		// Record the spend and deliver the chosen output.
		activation.Output = port
		r.visits(activation.GetNodeId()).Spend += spend
		r.deliver(activation.GetNodeId(), port, activation.GetTaskKey())
	}
	return nil
}

// outcome reads the output port a completed activation chose and the spend it
// reported. failure describes a Step failure; err is a World error.
func (r *runner) outcome(ctx context.Context, nodeID string, task *forge_task.Task) (
	port string,
	spend uint64,
	failure string,
	err error,
) {
	// Fail the Step when its Task failed.
	if result := task.GetResult(); !result.IsSuccessful() {
		result.FillFailError()
		return "", 0, "step failed: " + result.GetFailError(), nil
	}

	// Collect the set outputs that name output ports, and read the spend.
	node := r.graph.GetNodes()[nodeID]
	var ports []string
	for _, output := range task.GetValueSet().GetOutputs() {
		if output.IsEmpty() {
			continue
		}
		if output.GetName() == s4wave_flowgraph.SpendOutputName {
			spend, failure, err = r.readSpend(ctx, output)
			if failure != "" || err != nil {
				return "", 0, failure, err
			}
			continue
		}
		if node.FindPort(output.GetName()).GetDirection() == s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT {
			ports = append(ports, output.GetName())
		}
	}

	// Require exactly one chosen port.
	if len(ports) != 1 {
		return "", 0, "step set " + strconv.Itoa(len(ports)) + " output ports, want one", nil
	}
	return ports[0], spend, "", nil
}

// readSpend reads a spend output as a decimal integer blob.
func (r *runner) readSpend(ctx context.Context, output *forge_value.Value) (uint64, string, error) {
	// Read the blob the output references.
	ref, err := output.ToBucketRef()
	if err != nil {
		return 0, "spend: " + err.Error(), nil
	}
	var data []byte
	_, err = world.AccessObject(ctx, r.ws.AccessWorldState, ref, func(cursor *block.Cursor) error {
		var err error
		data, err = blob.FetchToBytes(ctx, cursor)
		return err
	})
	if err != nil {
		return 0, "", err
	}

	// Parse the decimal spend.
	spend, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, "spend: " + err.Error(), nil
	}
	return spend, "", nil
}

// deliver queues an output for each Step input connected to a node's port, in
// connection ID order, so a port with several connections forks the run.
func (r *runner) deliver(nodeID, port, taskKey string) {
	connections := r.graph.GetConnections()
	for _, id := range slices.Sorted(maps.Keys(connections)) {
		connection := connections[id]
		if connection.GetOutputNode() != nodeID || connection.GetOutputPort() != port {
			continue
		}
		if r.graph.GetNodes()[connection.GetInputNode()].GetStep() == nil {
			continue
		}
		r.run.Arrivals = append(r.run.Arrivals, &s4wave_flowgraph.FlowgraphArrival{
			NodeId:     connection.GetInputNode(),
			Port:       connection.GetInputPort(),
			TaskKey:    taskKey,
			OutputName: port,
		})
	}
}

// start activates each ready Step while the run is running. A Step is ready
// when every connected input port has an arrival, so a Step joins its inbound
// branches; a Step with no connected input is ready on a start arrival.
func (r *runner) start(ctx context.Context) error {
	nodes := r.graph.GetNodes()
	for _, id := range slices.Sorted(maps.Keys(nodes)) {
		// Skip nodes that are not Steps.
		step := nodes[id].GetStep()
		if step == nil {
			continue
		}

		// Activate the Step once per complete set of inputs.
		ports := inputPorts(r.graph, id)
		for r.run.GetState() == s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING {
			taken := r.ready(id, ports)
			if taken == nil {
				break
			}
			if reason := r.blocked(id, step); reason != "" {
				r.pause(id, reason)
				return nil
			}
			if err := r.activate(ctx, id, step, r.consume(taken)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ready returns the indexes of the arrivals that activate a Step, or nil when
// an input port has none.
func (r *runner) ready(nodeID string, ports []string) []int {
	// A Step without connected inputs takes one start arrival.
	if len(ports) == 0 {
		ports = []string{""}
	}

	// Take the earliest arrival at each port.
	taken := make([]int, 0, len(ports))
	for _, port := range ports {
		i := slices.IndexFunc(r.run.GetArrivals(), func(arrival *s4wave_flowgraph.FlowgraphArrival) bool {
			return arrival.GetNodeId() == nodeID && arrival.GetPort() == port
		})
		if i < 0 {
			return nil
		}
		taken = append(taken, i)
	}
	return taken
}

// consume removes the arrivals at the given indexes and returns them.
func (r *runner) consume(taken []int) []*s4wave_flowgraph.FlowgraphArrival {
	inputs := make([]*s4wave_flowgraph.FlowgraphArrival, 0, len(taken))
	for _, i := range taken {
		inputs = append(inputs, r.run.Arrivals[i])
	}
	r.run.Arrivals = slices.DeleteFunc(r.run.Arrivals, func(arrival *s4wave_flowgraph.FlowgraphArrival) bool {
		return slices.Contains(inputs, arrival)
	})
	return inputs
}

// blocked returns why a Step may not activate again, or empty when it may.
func (r *runner) blocked(nodeID string, step *s4wave_flowgraph.FlowgraphStep) string {
	// Require an executable Target.
	if step.GetTarget() == nil {
		return "step has no target"
	}
	if err := step.GetTarget().Validate(); err != nil {
		return "step target: " + err.Error()
	}

	// Pause at the Step's bound.
	bound, visits := step.GetBound(), r.run.GetVisits()[nodeID]
	if limit := bound.GetMaxVisits(); limit != 0 && visits.GetCount() >= limit {
		return "step reached its bound of " + strconv.FormatUint(uint64(limit), 10) + " visits"
	}
	if limit := bound.GetMaxSpend(); limit != 0 && visits.GetSpend() >= limit {
		return "step reached its bound of " + strconv.FormatUint(limit, 10) + " spend"
	}
	return ""
}

// activate adds a Task running the Step to the run's Job, with each input
// port wired to the output that arrived there.
func (r *runner) activate(
	ctx context.Context,
	nodeID string,
	step *s4wave_flowgraph.FlowgraphStep,
	inputs []*s4wave_flowgraph.FlowgraphArrival,
) error {
	// Wire each arrived output as a Task output input named after its port.
	target := step.GetTarget().CloneVT()
	for _, arrival := range inputs {
		if arrival.GetPort() == "" {
			continue
		}
		target.Inputs = slices.DeleteFunc(target.Inputs, func(input *forge_target.Input) bool {
			return input.GetName() == arrival.GetPort()
		})
		target.Inputs = append(target.Inputs, &forge_target.Input{
			Name:      arrival.GetPort(),
			InputType: forge_target.InputType_InputType_TASK_OUTPUT,
			TaskOutput: &forge_target.InputTaskOutput{
				TaskKey:    arrival.GetTaskKey(),
				OutputName: arrival.GetOutputName(),
			},
		})
	}

	// Create the Task with the Job's placement.
	jobKey := r.run.GetJobKey()
	job, err := forge_job.LookupJobBody(ctx, r.ws, jobKey)
	if err != nil {
		return err
	}
	name := "a" + strconv.Itoa(len(r.run.GetActivations()))
	taskKey := forge_job.NewJobTaskKey(jobKey, name)
	obj, _, err := forge_task.CreateTaskWithTarget(ctx, r.ws, "", taskKey, name, target, "", 1, job.GetPlacement(), timestamp.Now())
	world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}

	// Link the Task into the Job, which wakes the Job's Cluster.
	if err := world_parent.SetObjectParent(ctx, r.ws, taskKey, jobKey, false); err != nil {
		return err
	}
	if err := r.ws.SetGraphQuad(ctx, forge_job.NewJobToTaskQuad(jobKey, taskKey)); err != nil {
		return err
	}

	// Record the activation and count the visit.
	r.run.Activations = append(r.run.Activations, &s4wave_flowgraph.FlowgraphActivation{
		NodeId:  nodeID,
		TaskKey: taskKey,
		Inputs:  inputs,
	})
	r.visits(nodeID).Count++
	return nil
}

// settle completes a running run with nothing running or waiting, and pauses
// one whose waiting inputs can no longer meet.
func (r *runner) settle() {
	if r.run.GetState() != s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING || len(r.running()) != 0 {
		return
	}
	if arrivals := r.run.GetArrivals(); len(arrivals) != 0 {
		r.pause(arrivals[0].GetNodeId(), "step is missing inputs")
		return
	}
	r.run.State = s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_COMPLETE
}

// pause stops starting Steps. The first reason is kept.
func (r *runner) pause(nodeID, reason string) {
	if r.run.GetState() != s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING {
		return
	}
	r.run.State = s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_PAUSED
	r.run.PausedNode = nodeID
	r.run.PauseReason = reason
}

// visits returns a Step's counters, creating them on first use.
func (r *runner) visits(nodeID string) *s4wave_flowgraph.FlowgraphVisits {
	if r.run.Visits == nil {
		r.run.Visits = make(map[string]*s4wave_flowgraph.FlowgraphVisits)
	}
	visits := r.run.Visits[nodeID]
	if visits == nil {
		visits = &s4wave_flowgraph.FlowgraphVisits{}
		r.run.Visits[nodeID] = visits
	}
	return visits
}

// running returns the Task keys of activations not yet routed.
func (r *runner) running() []string {
	var keys []string
	for _, activation := range r.run.GetActivations() {
		if !activation.GetDone() {
			keys = append(keys, activation.GetTaskKey())
		}
	}
	return keys
}

// inputPorts returns the sorted input ports of a node that have a connection.
func inputPorts(graph *s4wave_flowgraph.Flowgraph, nodeID string) []string {
	var ports []string
	for _, connection := range graph.GetConnections() {
		if connection.GetInputNode() == nodeID && !slices.Contains(ports, connection.GetInputPort()) {
			ports = append(ports, connection.GetInputPort())
		}
	}
	slices.Sort(ports)
	return ports
}
