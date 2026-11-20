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
	task_tx "github.com/s4wave/spacewave/forge/task/tx"
	forge_value "github.com/s4wave/spacewave/forge/value"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// runner advances one run within a World transaction.
type runner struct {
	// runKey identifies the owning run.
	runKey string
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
	r := &runner{runKey: runKey, ws: ws, graph: graph, run: previous.CloneVT()}
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
// and pauses the run at its Step. Either way, the live inputs the activation
// never acknowledged return to the arrivals with their keys, so the next
// readiness pass starts a successor that receives them. A standing run then
// drops the activations it routed: their Tasks and the Job keep the record, so
// the run holds only the activations still running.
func (r *runner) route(ctx context.Context) error {
	if r.run.GetStanding() {
		defer func() {
			r.run.Activations = slices.DeleteFunc(r.run.Activations, (*s4wave_flowgraph.FlowgraphActivation).GetDone)
		}()
	}
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

		// Resolve the routing key before committing a successful outcome.
		key := activation.GetKey()
		if failure == "" {
			key, failure, err = r.routingKey(ctx, task, key)
			if err != nil {
				return err
			}
		}
		activation.Done = true
		if failure != "" {
			r.run.Arrivals = append(r.run.Arrivals, activation.GetInputs()...)
			r.returnInbox(activation)
			r.pause(activation.GetNodeId(), failure)
			continue
		}
		r.returnInbox(activation)

		// Record the spend and deliver the chosen output.
		activation.Output = port
		r.visits(activation.GetNodeId()).Spend += spend
		r.deliver(activation.GetNodeId(), port, activation.GetTaskKey(), key)
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
	data, err := r.readOutput(ctx, output)
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
func (r *runner) deliver(nodeID, port, taskKey, key string) {
	for _, connection := range outgoing(r.graph, nodeID) {
		if connection.GetOutputPort() != port {
			continue
		}
		r.run.Arrivals = append(r.run.Arrivals, &s4wave_flowgraph.FlowgraphArrival{
			NodeId: connection.GetInputNode(),
			Port:   connection.GetInputPort(),
			Key:    key,
			Source: &s4wave_flowgraph.FlowgraphArrival_TaskOutput{
				TaskOutput: &s4wave_flowgraph.FlowgraphTaskOutput{TaskKey: taskKey, OutputName: port},
			},
		})
	}
}

// outgoing returns the connections from a node into Step inputs, in
// connection ID order.
func outgoing(graph *s4wave_flowgraph.Flowgraph, nodeID string) []*s4wave_flowgraph.FlowgraphConnection {
	var out []*s4wave_flowgraph.FlowgraphConnection
	connections := graph.GetConnections()
	for _, id := range slices.Sorted(maps.Keys(connections)) {
		connection := connections[id]
		if connection.GetOutputNode() == nodeID && graph.GetNodes()[connection.GetInputNode()].GetStep() != nil {
			out = append(out, connection)
		}
	}
	return out
}

// start activates each ready Step while the run is running. A Step is ready
// when every connected input port has an arrival, so a Step joins its inbound
// branches; a Step with no connected input is ready on a start arrival. A
// keyed Step is ready per key: its ports are met by arrivals that carry one
// key, and an arrival for a key with a running activation goes to that
// activation's inbox, in any run state.
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
		for {
			if step.GetKeyed() {
				r.deliverLive(id)
			}
			if r.run.GetState() != s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING {
				break
			}
			taken, key := r.ready(id, ports, step.GetKeyed())
			if taken == nil {
				break
			}
			if reason := r.blocked(id, step); reason != "" {
				r.pause(id, reason)
				break
			}
			if err := r.activate(ctx, id, step, key, r.consume(taken)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ready returns the indexes of the arrivals that activate a Step and the key
// they share, or nil when an input port has none. An unkeyed Step takes the
// earliest arrival at each port, whatever its key, and returns no key. A
// keyed Step takes the earliest key, in arrival order, that has an arrival at
// every port.
func (r *runner) ready(nodeID string, ports []string, keyed bool) ([]int, string) {
	// A Step without connected inputs takes one start arrival.
	if len(ports) == 0 {
		ports = []string{""}
	}

	// Index the Step's earliest arrival at each port, by key for a keyed Step
	// and under one empty key otherwise, keeping the keys in arrival order.
	var tries []string
	earliest := make(map[string]map[string]int)
	for i, arrival := range r.run.GetArrivals() {
		if arrival.GetNodeId() != nodeID {
			continue
		}
		var key string
		if keyed {
			key = arrival.GetKey()
		}
		byPort := earliest[key]
		if byPort == nil {
			byPort = make(map[string]int)
			earliest[key] = byPort
			tries = append(tries, key)
		}
		if _, ok := byPort[arrival.GetPort()]; !ok {
			byPort[arrival.GetPort()] = i
		}
	}

	// Take the first key with an arrival at every port.
	for _, key := range tries {
		taken := make([]int, 0, len(ports))
		for _, port := range ports {
			i, ok := earliest[key][port]
			if !ok {
				break
			}
			taken = append(taken, i)
		}
		if len(taken) == len(ports) {
			return taken, key
		}
	}
	return nil, ""
}

// consume removes the arrivals at the given indexes and returns them.
func (r *runner) consume(taken []int) []*s4wave_flowgraph.FlowgraphArrival {
	// Collect the taken arrivals, then remove them from the queue.
	inputs := make([]*s4wave_flowgraph.FlowgraphArrival, 0, len(taken))
	drop := make(map[*s4wave_flowgraph.FlowgraphArrival]bool, len(taken))
	for _, i := range taken {
		inputs = append(inputs, r.run.Arrivals[i])
		drop[r.run.Arrivals[i]] = true
	}
	r.run.Arrivals = slices.DeleteFunc(r.run.Arrivals, func(arrival *s4wave_flowgraph.FlowgraphArrival) bool {
		return drop[arrival]
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
// port wired to the output or event object that arrived there. key is the key
// of a keyed Step's activation, and empty for an unkeyed Step.
func (r *runner) activate(
	ctx context.Context,
	nodeID string,
	step *s4wave_flowgraph.FlowgraphStep,
	key string,
	inputs []*s4wave_flowgraph.FlowgraphArrival,
) error {
	// Wire each arrival as a Forge input named after its port.
	target := step.GetTarget().CloneVT()
	for _, arrival := range inputs {
		input := arrivalInput(arrival)
		if input == nil {
			continue
		}
		target.Inputs = slices.DeleteFunc(target.Inputs, func(existing *forge_target.Input) bool {
			return existing.GetName() == input.GetName()
		})
		target.Inputs = append(target.Inputs, input)
	}

	// Resolve the Job placement and reserve the next activation identity.
	jobKey := r.run.GetJobKey()
	job, err := forge_job.LookupJobBody(ctx, r.ws, jobKey)
	if err != nil {
		return err
	}
	name, taskKey, err := r.newTaskName(ctx, jobKey)
	if err != nil {
		return err
	}

	// Supply the activation context before persisting the Task Target.
	contextValue, err := r.contextInput(ctx, target, taskKey, key, nodeID)
	if err != nil {
		return err
	}
	obj, _, err := forge_task.CreateTaskWithTarget(ctx, r.ws, "", taskKey, name, target, "", 1, job.GetPlacement(), timestamp.Now())
	world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}

	// Retain the context as a scheduler-supplied Task input before publishing
	// the activation. Forge carries this ValueSet through its Pass and Execution.
	update := task_tx.NewTxUpdateInputs(taskKey)
	update.TxUpdateInputs.ValueSet = &forge_target.ValueSet{Inputs: []*forge_value.Value{contextValue}}
	if _, _, err := r.ws.ApplyWorldOp(ctx, update, ""); err != nil {
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
		Key:     key,
	})
	r.visits(nodeID).Count++
	return nil
}

// newTaskName returns the name and key of the Job's next Task. A standing run
// drops routed activations, so the count of activations alone would reuse a
// name: the search starts at the larger of that count and the visits, and
// skips names whose Task exists.
func (r *runner) newTaskName(ctx context.Context, jobKey string) (string, string, error) {
	// Start beyond every activation already counted by the run.
	n := len(r.run.GetActivations())
	var visits int
	for _, v := range r.run.GetVisits() {
		visits += int(v.GetCount())
	}
	n = max(n, visits)

	// Skip durable Tasks retained by the Job after the run drops their activations.
	for {
		name := "a" + strconv.Itoa(n)
		taskKey := forge_job.NewJobTaskKey(jobKey, name)
		obj, found, err := r.ws.GetObject(ctx, taskKey)
		world.ReleaseObjectState(obj)
		if err != nil {
			return "", "", err
		}
		if !found {
			return name, taskKey, nil
		}
		n++
	}
}

// arrivalInput returns the Forge input that reads an arrival at its port: the
// Task output for a Task-output arrival, and the World object for an event
// arrival, which the Task reads as it reads any WORLD_OBJECT input. The event
// ID stays in the run's record of the activation's inputs. A start arrival has
// no port and no input, so it returns nil.
func arrivalInput(arrival *s4wave_flowgraph.FlowgraphArrival) *forge_target.Input {
	port := arrival.GetPort()
	if port == "" {
		return nil
	}
	if event := arrival.GetEvent(); event != nil {
		return &forge_target.Input{
			Name:      port,
			InputType: forge_target.InputType_InputType_WORLD_OBJECT,
			WorldObject: &forge_target.InputWorldObject{
				ObjectKey: event.GetObjectKey(),
			},
		}
	}
	return &forge_target.Input{
		Name:      port,
		InputType: forge_target.InputType_InputType_TASK_OUTPUT,
		TaskOutput: &forge_target.InputTaskOutput{
			TaskKey:    arrival.GetTaskOutput().GetTaskKey(),
			OutputName: arrival.GetTaskOutput().GetOutputName(),
		},
	}
}

// settle completes a running run with nothing running or waiting, and pauses
// one whose waiting inputs can no longer meet. Inputs waiting at a Step that a
// branch not taken left unable to run are dropped first, so a run whose
// decision routed away from a join completes. A standing run does neither: it
// is never COMPLETE, and an arrival waiting at a joined Step is the next
// event's partner, not a stuck input.
func (r *runner) settle() {
	// Settle only a running run with nothing running, never a standing one.
	if r.run.GetStanding() || r.run.GetState() != s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING || len(r.running()) != 0 {
		return
	}

	// Drop the inputs of each Step a branch not taken left unable to run.
	filled := make(map[string]map[string]bool)
	for _, arrival := range r.run.GetArrivals() {
		id := arrival.GetNodeId()
		if filled[id] == nil {
			filled[id] = make(map[string]bool)
		}
		filled[id][arrival.GetPort()] = true
	}
	stranded := make(map[string]bool, len(filled))
	for id, ports := range filled {
		stranded[id] = r.stranded(id, ports)
	}
	r.run.Arrivals = slices.DeleteFunc(r.run.GetArrivals(), func(arrival *s4wave_flowgraph.FlowgraphArrival) bool {
		return stranded[arrival.GetNodeId()]
	})

	// Pause on any other input that waits, else complete.
	if arrivals := r.run.GetArrivals(); len(arrivals) != 0 {
		r.pause(arrivals[0].GetNodeId(), "step is missing inputs")
		return
	}
	r.run.State = s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_COMPLETE
}

// stranded reports whether a Step, whose waiting arrivals fill the ports in
// filled, waits on an input port that a branch not taken will never fill:
// every node feeding the port has activated, and none of its activations chose
// the output that feeds it. A port missing for another reason, such as a
// feeding node that never ran, leaves the Step waiting, so the run pauses
// there.
func (r *runner) stranded(nodeID string, filled map[string]bool) bool {
	for _, port := range inputPorts(r.graph, nodeID) {
		if !filled[port] && r.notTaken(nodeID, port) {
			return true
		}
	}
	return false
}

// notTaken reports whether every connection into a Step's input port comes
// from a node whose activations all routed an output, none of them the
// connection's. A failed activation routed nothing.
func (r *runner) notTaken(nodeID, port string) bool {
	for _, connection := range r.graph.GetConnections() {
		if connection.GetInputNode() != nodeID || connection.GetInputPort() != port {
			continue
		}

		// Require the feeding node to have run and routed elsewhere.
		ran := false
		for _, activation := range r.run.GetActivations() {
			if activation.GetNodeId() != connection.GetOutputNode() {
				continue
			}
			output := activation.GetOutput()
			if !activation.GetDone() || output == "" || output == connection.GetOutputPort() {
				return false
			}
			ran = true
		}
		if !ran {
			return false
		}
	}
	return true
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
		if connection.GetInputNode() == nodeID {
			ports = append(ports, connection.GetInputPort())
		}
	}
	slices.Sort(ports)
	return slices.Compact(ports)
}

// readOutput reads a reserved output's blob through its storage reference.
func (r *runner) readOutput(ctx context.Context, output *forge_value.Value) ([]byte, error) {
	// Resolve the Forge value to its stored object.
	ref, err := output.ToBucketRef()
	if err != nil {
		return nil, err
	}

	// Read the output blob without changing it.
	var data []byte
	_, err = world.AccessObject(ctx, r.ws.AccessWorldState, ref, func(cursor *block.Cursor) error {
		var err error
		data, err = blob.FetchToBytes(ctx, cursor)
		return err
	})
	return data, err
}

// routingKey reads the reserved key output, or inherits the activation's key.
func (r *runner) routingKey(ctx context.Context, task *forge_task.Task, inherited string) (string, string, error) {
	// An absent output inherits; an explicitly empty blob clears the key.
	output, _ := task.GetValueSet().LookupOutput(s4wave_flowgraph.KeyOutputName)
	if output == nil || output.IsEmpty() {
		return inherited, "", nil
	}
	data, err := r.readOutput(ctx, output)
	if err != nil {
		return "", "", err
	}

	// Permit one terminal line ending, but reject multiline routing keys.
	key := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if strings.ContainsAny(key, "\r\n") {
		return "", "flowgraph/key must be a one-line blob", nil
	}
	return key, "", nil
}
