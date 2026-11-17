package flowgraph_run

import (
	"context"
	"maps"
	"slices"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	world_expect "github.com/s4wave/spacewave/db/world/expect"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_task "github.com/s4wave/spacewave/forge/task"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_pure "github.com/s4wave/spacewave/sdk/flowgraph/pure"
	"github.com/sirupsen/logrus"
)

// pureKey identifies the watcher of one pure Step. The function ID is part of
// the key so a graph that changes the function restarts the watcher.
type pureKey struct {
	// nodeID is the pure Step's node.
	nodeID string
	// functionID is the function the Step evaluates.
	functionID string
}

// pureKeys returns a key for each pure Step of the graph.
func pureKeys(graph *s4wave_flowgraph.Flowgraph) []pureKey {
	var keys []pureKey
	nodes := graph.GetNodes()
	for _, id := range slices.Sorted(maps.Keys(nodes)) {
		if pure := nodes[id].GetStep().GetPure(); pure != nil {
			keys = append(keys, pureKey{nodeID: id, functionID: pure.GetFunctionId()})
		}
	}
	return keys
}

// pureReady reports whether a pure Step has a complete set of inputs.
func (r *runner) pureReady(nodeID string, step *s4wave_flowgraph.FlowgraphStep, ports []string) bool {
	taken, _ := r.ready(nodeID, ports, step.GetKeyed())
	return taken != nil
}

// anyPureReady reports whether any pure Step of the graph is ready.
func (r *runner) anyPureReady() bool {
	for id, node := range r.graph.GetNodes() {
		if step := node.GetStep(); step.GetPure() != nil && r.pureReady(id, step, inputPorts(r.graph, id)) {
			return true
		}
	}
	return false
}

// pureEval is one evaluation of a pure Step: the arrivals and state root it
// read, and the request built from them. The commit applies the result only
// when the run still holds the arrivals and the state object holds that root.
type pureEval struct {
	// key is the arrivals' shared key, and empty for an unkeyed Step.
	key string
	// inputs are the arrivals the evaluation consumes, in port order.
	inputs []*s4wave_flowgraph.FlowgraphArrival
	// stateKey is the state object's key, or empty when the Step keeps none.
	stateKey string
	// stateRoot is the state object's root, Absent when it did not exist.
	stateRoot *world_expect.ExpectedRoot
	// req is the evaluation request.
	req *flowgraph_pure.EvaluateRequest
	// fault says why the inputs cannot be evaluated, or is empty.
	fault string
}

// readPure reads a ready pure Step's evaluation from the run, or returns nil
// when the Step is not ready: the run is not running, the Step changed, its
// inputs are incomplete, or its bound holds it.
func (r *runner) readPure(ctx context.Context, key pureKey) (*pureEval, error) {
	// Require a running run whose Step still evaluates the function.
	step := r.graph.GetNodes()[key.nodeID].GetStep()
	pure := step.GetPure()
	if r.run.GetState() != s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING || pure.GetFunctionId() != key.functionID {
		return nil, nil
	}

	// Take the ready arrivals unless the Step's bound holds it.
	taken, arrivalKey := r.ready(key.nodeID, inputPorts(r.graph, key.nodeID), step.GetKeyed())
	if taken == nil || r.blocked(key.nodeID, step) != "" {
		return nil, nil
	}
	eval := &pureEval{
		key: arrivalKey,
		req: &flowgraph_pure.EvaluateRequest{FunctionId: pure.GetFunctionId(), Config: pure.GetConfig()},
	}
	for _, i := range taken {
		arrival := r.run.GetArrivals()[i]
		input, failure, err := r.pureInput(ctx, arrival)
		if err != nil {
			return nil, err
		}
		if failure != "" && eval.fault == "" {
			eval.fault = failure
		}
		eval.inputs = append(eval.inputs, arrival.CloneVT())
		if failure == "" {
			eval.req.Inputs = append(eval.req.Inputs, input)
		}
	}

	// Read the state object and its root.
	if prefix := pure.GetStatePrefix(); prefix != "" {
		eval.stateKey = prefix + arrivalKey
		state, root, err := r.readState(ctx, eval.stateKey)
		if err != nil {
			return nil, err
		}
		eval.stateRoot, eval.req.State = root, state
	}
	return eval, nil
}

// pureInput builds the function input for an arrival, reading a Task output's
// blob through its reference. failure says why the arrival cannot be an input,
// which faults the Step.
func (r *runner) pureInput(ctx context.Context, arrival *s4wave_flowgraph.FlowgraphArrival) (*flowgraph_pure.PureInput, string, error) {
	// Describe the arrival.
	input := &flowgraph_pure.PureInput{Port: arrival.GetPort(), Key: arrival.GetKey()}
	switch source := arrival.GetSource().(type) {
	case *s4wave_flowgraph.FlowgraphArrival_Event:
		input.EventId = source.Event.GetEventId()
		input.SourcePort = source.Event.GetPort()
		input.Value = source.Event.GetPayload()
	case *s4wave_flowgraph.FlowgraphArrival_StepOutput:
		input.SourcePort = source.StepOutput.GetOutputName()
		input.Value = source.StepOutput.GetValue()
	case *s4wave_flowgraph.FlowgraphArrival_TaskOutput:
		input.SourcePort = source.TaskOutput.GetOutputName()
	}

	// Find a Task output's value in the Task's outputs.
	taskOutput := arrival.GetTaskOutput()
	if taskOutput == nil {
		return input, "", nil
	}
	task, err := forge_task.LookupTaskBody(ctx, r.ws, taskOutput.GetTaskKey())
	if err != nil {
		return nil, "", err
	}
	output, _ := task.GetValueSet().LookupOutput(taskOutput.GetOutputName())
	if output == nil || output.IsEmpty() {
		return input, "", nil
	}

	// Read its blob; an oversized output faults the Step.
	value, failure, err := r.readOutput(ctx, output, taskOutput.GetOutputName())
	if err != nil || failure != "" {
		return nil, failure, err
	}
	input.Value = value
	return input, "", nil
}

// readState reads a state object's blob and root block. A missing object
// reads as a nil root with no body.
func (r *runner) readState(ctx context.Context, key string) ([]byte, *world_expect.ExpectedRoot, error) {
	// Look up the state object and its root.
	obj, found, err := r.ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, &world_expect.ExpectedRoot{ObjectKey: key, Absent: true}, nil
	}
	ref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Read the body.
	var data []byte
	_, _, err = world.AccessObjectState(ctx, obj, false, func(cursor *block.Cursor) error {
		var err error
		data, err = blob.FetchToBytes(ctx, cursor)
		return err
	})
	return data, &world_expect.ExpectedRoot{ObjectKey: key, RootRef: ref.GetRootRef()}, err
}

// commitPure applies an evaluation's result in the caller's write
// transaction. It returns false, changing nothing, when the evaluation lost a
// race: the run no longer holds the evaluated arrivals at a running, unbounded
// Step, or the state object moved. The transaction expects the run and the
// state object to keep the roots it read, so a replay on another head is
// refused and the watcher evaluates again. A fault pauses the run at the Step and keeps the
// arrivals. A conclusion consumes the arrivals, writes the next state, counts
// the visit and routes the chosen port with the arrivals' key.
func commitPure(
	ctx context.Context,
	ws world.WorldState,
	runKey string,
	key pureKey,
	eval *pureEval,
	resp *flowgraph_pure.EvaluateResponse,
) (bool, error) {
	// Read the run and recheck what the evaluation read.
	obj, previous, graph, err := readRun(ctx, ws, runKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return false, err
	}
	r := &runner{runKey: runKey, ws: ws, graph: graph, run: previous.CloneVT()}
	taken, err := r.pureCurrent(ctx, key, eval)
	if err != nil || taken == nil {
		return false, err
	}

	// pureCurrent checked the evaluation's state root against this head;
	// the expectation pins that root for a replay onto another head.
	if eval.stateKey != "" {
		if err := world_expect.ExpectCurrent(ctx, ws, eval.stateKey); err != nil {
			return false, err
		}
	}

	// Pause on a fault or a port the Step does not declare.
	concluded := resp.GetConcluded()
	failure := resp.GetFault().GetReason()
	if err := resp.Validate(); err != nil {
		failure = err.Error()
	}
	direction := graph.GetNodes()[key.nodeID].FindPort(concluded.GetPort()).GetDirection()
	if failure == "" && direction != s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT {
		failure = "chose undeclared output port " + concluded.GetPort()
	}
	if failure != "" {
		r.pause(key.nodeID, "pure function "+key.functionID+": "+failure)
	} else {
		// Consume the arrivals, write the state, and route the output.
		r.consume(taken)
		if err := writeState(ctx, ws, eval.stateKey, graph.GetNodes()[key.nodeID].GetStep().GetPure().GetStateTypeId(), concluded.GetNextState()); err != nil {
			return false, err
		}
		r.visits(key.nodeID).Count++
		r.deliver(key.nodeID, concluded.GetPort(), eval.key, &s4wave_flowgraph.FlowgraphArrival{
			Source: &s4wave_flowgraph.FlowgraphArrival_StepOutput{
				StepOutput: &s4wave_flowgraph.FlowgraphStepOutput{
					NodeId:     key.nodeID,
					OutputName: concluded.GetPort(),
					Value:      concluded.GetOutput(),
				},
			},
		})
	}

	// Write the run; the run controller advances from its new revision.
	err = writeRun(ctx, ws, runKey, obj, func(cursor *block.Cursor) {
		cursor.SetBlock(r.run, true)
	})
	return err == nil, err
}

// pureCurrent returns the indexes of the evaluated arrivals in the run when
// the evaluation still applies, or nil when it lost a race. The run must still
// be running the Step's function below its bound, each evaluated arrival must
// match the content of a distinct arrival of the run at any index, and the
// state object must hold the root read. Matching content suffices because the
// function is pure: equal arrivals on an equal state give the same result, and
// the state root tells a fold already applied from one still to apply. It
// compares roots and reads no block, since it runs under the World writer.
func (r *runner) pureCurrent(ctx context.Context, key pureKey, eval *pureEval) ([]int, error) {
	// Require the running Step with the same function and an open bound.
	step := r.graph.GetNodes()[key.nodeID].GetStep()
	if r.run.GetState() != s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING ||
		step.GetPure().GetFunctionId() != key.functionID || r.blocked(key.nodeID, step) != "" {
		return nil, nil
	}

	// Find each evaluated arrival at its own index.
	taken := make([]int, 0, len(eval.inputs))
	used := make([]bool, len(r.run.GetArrivals()))
	for _, input := range eval.inputs {
		i := -1
		for j, arrival := range r.run.GetArrivals() {
			if !used[j] && arrival.EqualVT(input) {
				i = j
				break
			}
		}
		if i < 0 {
			return nil, nil
		}
		used[i] = true
		taken = append(taken, i)
	}

	// Require the state object to hold the root read.
	if eval.stateKey != "" {
		root, err := world_expect.ReadRoot(ctx, r.ws, eval.stateKey)
		if err != nil || !root.Matches(eval.stateRoot) {
			return nil, err
		}
	}
	return taken, nil
}

// writeState writes the next state body to the state object, creating it with
// its type on first use. An empty key means the Step keeps no state.
func writeState(ctx context.Context, ws world.WorldState, key, typeID string, next []byte) error {
	// Skip a Step without state.
	if key == "" {
		return nil
	}
	build := func(cursor *block.Cursor) error {
		_, err := blob.BuildBlobWithBytes(ctx, next, cursor)
		return err
	}

	// Update an existing object.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if found {
		_, _, err = world.AccessObjectState(ctx, obj, true, build)
		return err
	}

	// Create the object and set its type.
	created, _, err := world.CreateWorldObject(ctx, ws, key, build)
	world.ReleaseObjectState(created)
	if err != nil || typeID == "" {
		return err
	}
	return world_types.SetObjectType(ctx, ws, key, typeID)
}

// pureWatcher evaluates one pure Step of the run. It watches the run object
// and, on each revision with the Step ready, evaluates the Step's function
// outside any World transaction and commits the result in one. Watchers of
// the same Step on several Devices race; the commit's guard lets one win and
// the others evaluate again.
type pureWatcher struct {
	// c is the run controller.
	c *Controller
	// key identifies the Step.
	key pureKey
	// loop watches the run object.
	loop *world_control.WatchLoop
	// fn is the looked-up function, or nil before the first ready revision.
	fn flowgraph_pure.Function
	// fnRef holds the function's lookup while fn is set.
	fnRef directive.Reference
}

// newPureWatcher constructs the watcher routine for a pure Step.
func (c *Controller) newPureWatcher(key pureKey) (keyed.Routine, *pureWatcher) {
	w := &pureWatcher{c: c, key: key}
	w.loop = world_control.NewWatchLoop(
		c.le.WithField("object-loop", "flowgraph-run-pure").WithField("node", key.nodeID),
		c.conf.GetObjectKey(),
		w.processState,
	)
	return w.execute, w
}

// execute watches the run until the Step leaves the graph, then releases the
// function.
func (w *pureWatcher) execute(ctx context.Context) error {
	defer w.releaseFunction()
	return w.loop.Execute(ctx, world.NewEngineWorldState(w.c.engine, false))
}

// releaseFunction drops the function so the next evaluation looks it up again.
func (w *pureWatcher) releaseFunction() {
	if w.fnRef != nil {
		w.fnRef.Release()
	}
	w.fn, w.fnRef = nil, nil
}

// processState evaluates the Step when the run holds its inputs.
func (w *pureWatcher) processState(
	ctx context.Context,
	le *logrus.Entry,
	_ world.WorldState,
	obj world.ObjectState,
	_ *bucket.ObjectRef,
	_ uint64,
) (bool, error) {
	// Read the evaluation from one consistent read of the run.
	if obj == nil {
		return true, nil
	}
	eval, err := w.read(ctx)
	if err != nil || eval == nil {
		return true, err
	}

	// Evaluate outside any transaction, or fault inputs that cannot be.
	resp, err := w.evaluate(ctx, le, eval)
	if err != nil {
		return true, err
	}

	// Commit the result; a lost race evaluates again.
	var committed bool
	err = world.ExecTransaction(ctx, w.c.engine, true, func(ctx context.Context, tx world.WorldState) error {
		var err error
		committed, err = commitPure(ctx, tx, w.c.conf.GetObjectKey(), w.key, eval, resp)
		return err
	})
	if err != nil {
		return true, err
	}
	if !committed {
		le.Debug("pure evaluation lost a race, evaluating again")
		w.loop.Wake()
	}
	return true, nil
}

// evaluate returns the function's response to the evaluation, looking the
// function up first and waiting while no component registers it. A failed call
// drops the function, so the retry looks it up again.
func (w *pureWatcher) evaluate(ctx context.Context, le *logrus.Entry, eval *pureEval) (*flowgraph_pure.EvaluateResponse, error) {
	// Fault inputs the function cannot take.
	if eval.fault != "" {
		return &flowgraph_pure.EvaluateResponse{Result: &flowgraph_pure.EvaluateResponse_Fault{
			Fault: &flowgraph_pure.PureFault{Reason: eval.fault},
		}}, nil
	}

	// Look up the function, waiting while no component registers it.
	if w.fn == nil {
		le.Debugf("waiting for pure function %q", w.key.functionID)
		var err error
		w.fn, w.fnRef, err = flowgraph_pure.ExLookupPureFunction(ctx, w.c.bus, w.key.functionID)
		if err != nil {
			return nil, err
		}
	}

	// Call the function.
	resp, err := w.fn.Evaluate(ctx, eval.req)
	if err != nil {
		w.releaseFunction()
		return nil, errors.Wrapf(err, "evaluate pure function %q", w.key.functionID)
	}
	return resp, nil
}

// read reads the Step's evaluation in one read transaction.
func (w *pureWatcher) read(ctx context.Context) (*pureEval, error) {
	// Open the read transaction.
	tx, err := w.c.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Read the run and the Step's ready inputs and state.
	obj, run, graph, err := readRun(ctx, tx, w.c.conf.GetObjectKey())
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	r := &runner{runKey: w.c.conf.GetObjectKey(), ws: tx, graph: graph, run: run}
	return r.readPure(ctx, w.key)
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = (*pureWatcher)(nil).processState
