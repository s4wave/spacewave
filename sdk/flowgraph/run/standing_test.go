package flowgraph_run_test

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_expect "github.com/s4wave/spacewave/db/world/expect"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_world "github.com/s4wave/spacewave/forge/world"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_pure "github.com/s4wave/spacewave/sdk/flowgraph/pure"
	flowgraph_run "github.com/s4wave/spacewave/sdk/flowgraph/run"
)

// standingKey is the key of the test Flowgraph's standing run.
var standingKey = flowgraph_run.StandingRunKey(graphKey)

// standingRun finds or creates the test Flowgraph's standing run in one World
// transaction and returns its key.
func (e *env) standingRun() string {
	var key string
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		var err error
		key, err = flowgraph_run.StandingRun(ctx, tx, e.sender, graphKey, clusterKey, nil)
		return err
	})
	return key
}

// readStanding reads the standing run, or nil before it exists.
func (e *env) readStanding() *s4wave_flowgraph.FlowgraphRun {
	obj, found, err := e.tb.WorldState.GetObject(e.ctx, standingKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		e.t.Fatal(err)
	}
	if !found {
		return nil
	}
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

// deliverStanding delivers events from the source node to the standing run.
func (e *env) deliverStanding(cursor, next string, events ...*flowgraph_run.DeliveredEvent) flowgraph_run.DeliverEventsResult {
	var result flowgraph_run.DeliverEventsResult
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		var err error
		result, err = flowgraph_run.DeliverEvents(ctx, tx, standingKey, &flowgraph_run.DeliverEventsRequest{
			NodeId:     sourceID,
			Cursor:     []byte(cursor),
			NextCursor: []byte(next),
			Events:     events,
		})
		return err
	})
	return result
}

// waitStanding waits until both Steps have run the given number of times and
// the standing run holds no activation, and returns the run.
func (e *env) waitStanding(visits uint32) *s4wave_flowgraph.FlowgraphRun {
	e.t.Helper()
	var run *s4wave_flowgraph.FlowgraphRun
	e.waitFor("standing run visits", func() bool {
		run = e.readStanding()
		return run.GetVisits()["a"].GetCount() == visits && run.GetVisits()["b"].GetCount() == visits && len(run.GetActivations()) == 0
	})
	return run
}

// TestStandingRun delivers events across two Steps of a standing run and
// checks that the run keeps no done activation, never completes, and leaves
// the Job's Tasks in place.
func TestStandingRun(t *testing.T) {
	// Create a graph of an event source feeding a Step that feeds a second Step.
	e := newEnv(t)
	source := &s4wave_flowgraph.FlowgraphNode{
		TypeId: "test/event-source",
		Ports:  []*s4wave_flowgraph.FlowgraphPort{port("events", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT)},
	}
	e.createGraph(map[string]*s4wave_flowgraph.FlowgraphNode{
		sourceID: source,
		"a":      e.step([]string{"in"}, [2]string{"out", "none"}, true, nil),
		"b":      e.step([]string{"in"}, [2]string{"done", "none"}, true, nil),
	}, map[string]*s4wave_flowgraph.FlowgraphConnection{
		"src-a": connect(sourceID, "events", "a", "in"),
		"a-b":   connect("a", "out", "b", "in"),
	})
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, tx, eventObjectKey, func(cursor *block.Cursor) error {
			cursor.SetBlock(&s4wave_flowgraph.FlowgraphVisits{}, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		return err
	})

	// A first find creates the run, and a second finds the same one.
	if _, found, err := flowgraph_run.FindStandingRun(e.ctx, e.tb.WorldState, graphKey); err != nil || found {
		t.Fatalf("find before create = %v, %v; want not found", found, err)
	}
	key := e.standingRun()
	if key != standingKey {
		t.Fatalf("standing run key = %q, want %q", key, standingKey)
	}
	created := e.readStanding()
	if !created.GetStanding() || created.GetState() != s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING {
		t.Fatalf("created run = %v, want a running standing run", created)
	}
	if again := e.standingRun(); again != key {
		t.Fatalf("second find = %q, want %q", again, key)
	}
	if !e.readStanding().EqualVT(created) {
		t.Fatal("second find changed the run")
	}
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		if err := flowgraph_run.StartRun(ctx, tx, e.sender, "other-run", graphKey, clusterKey, nil, flowgraph_run.Standing()); err == nil {
			t.Error("standing StartRun under another key succeeded")
		}
		return nil
	})

	// Run the standing run and deliver an event, which crosses both Steps.
	e.startWorker()
	_, ref, err := flowgraph_run.StartControllerWithConfig(e.ctx, e.tb.Bus, flowgraph_run.NewConfig(e.tb.EngineID, key))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
	if result := e.deliverStanding("", "c1", event("e1", "")); result != flowgraph_run.DeliverEventsResult_DELIVER_EVENTS_RESULT_DELIVERED {
		t.Fatalf("first delivery = %s", result)
	}
	run := e.waitStanding(1)
	if run.GetState() != s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING {
		t.Fatalf("standing run state = %s, want running with nothing waiting", run.GetState())
	}

	// A second event crosses both Steps again, and the run still holds no activation.
	if result := e.deliverStanding("c1", "c2", event("e2", "")); result != flowgraph_run.DeliverEventsResult_DELIVER_EVENTS_RESULT_DELIVERED {
		t.Fatalf("second delivery = %s", result)
	}
	run = e.waitStanding(2)
	if run.GetState() != s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING || len(run.GetArrivals()) != 0 {
		t.Fatalf("standing run = %v, want running with no arrivals", run)
	}
	if got := string(run.GetCursor()[sourceID]); got != "c2" {
		t.Fatalf("cursor = %q, want c2", got)
	}

	// The Job keeps all four Tasks, each complete and under its own key.
	taskKeys, err := forge_job.ListJobTasks(e.ctx, e.tb.WorldState, flowgraph_run.JobKey(key))
	if err != nil {
		t.Fatal(err)
	}
	if len(taskKeys) != 4 {
		t.Fatalf("job tasks = %v, want 4", taskKeys)
	}
	for _, taskKey := range taskKeys {
		task, err := forge_task.LookupTaskBody(e.ctx, e.tb.WorldState, taskKey)
		if err != nil {
			t.Fatal(err)
		}
		if !task.IsComplete() {
			t.Fatalf("task %s is not complete", taskKey)
		}
	}
}

// foldFunction is a pure function that sums the decimal payloads of its
// inputs into a decimal state and routes the sum along "out". The first two
// evaluations wait for each other, so two run controllers evaluate the first
// event together and race to commit it.
type foldFunction struct {
	// barrier is closed when the second evaluation starts.
	barrier chan struct{}
	// mtx guards calls.
	mtx sync.Mutex
	// calls counts the evaluations.
	calls int
}

// Evaluate adds each input's payload to the state.
func (f *foldFunction) Evaluate(ctx context.Context, req *flowgraph_pure.EvaluateRequest) (*flowgraph_pure.EvaluateResponse, error) {
	// Hold the first evaluation until the second starts.
	f.mtx.Lock()
	f.calls++
	calls := f.calls
	if calls == 2 {
		close(f.barrier)
	}
	f.mtx.Unlock()
	if calls <= 2 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-f.barrier:
		}
	}

	// Sum the state and the payloads.
	sum := 0
	if len(req.GetState()) != 0 {
		n, err := strconv.Atoi(string(req.GetState()))
		if err != nil {
			return nil, err
		}
		sum = n
	}
	for _, input := range req.GetInputs() {
		n, err := strconv.Atoi(string(input.GetValue()))
		if err != nil {
			return &flowgraph_pure.EvaluateResponse{Result: &flowgraph_pure.EvaluateResponse_Fault{
				Fault: &flowgraph_pure.PureFault{Reason: "payload is not a number"},
			}}, nil
		}
		sum += n
	}
	next := []byte(strconv.Itoa(sum))
	return &flowgraph_pure.EvaluateResponse{Result: &flowgraph_pure.EvaluateResponse_Concluded{
		Concluded: &flowgraph_pure.PureConcluded{Port: "out", Output: next, NextState: next},
	}}, nil
}

// pureFunctions registers one pure function on the bus.
type pureFunctions struct {
	// id is the function ID.
	id string
	// fn is the function.
	fn flowgraph_pure.Function
}

// GetControllerInfo returns information about the controller.
func (c *pureFunctions) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/pure-functions", controller.MustParseVersion("0.0.1"), "pure functions")
}

// Execute returns at once, since the controller only handles directives.
func (c *pureFunctions) Execute(context.Context) error {
	return nil
}

// HandleDirective answers the lookup of the function.
func (c *pureFunctions) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(flowgraph_pure.LookupPureFunction)
	if !ok || dir.LookupPureFunctionID() != c.id {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]flowgraph_pure.LookupPureFunctionValue{c.fn}), nil)
}

// Close releases nothing.
func (c *pureFunctions) Close() error {
	return nil
}

// readState reads a state object's body and type.
func (e *env) readState(key string) (string, string) {
	e.t.Helper()
	var data []byte
	obj, found, err := e.tb.WorldState.GetObject(e.ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil || !found {
		e.t.Fatalf("state object %s: found %v, %v", key, found, err)
	}
	_, _, err = world.AccessObjectState(e.ctx, obj, false, func(cursor *block.Cursor) error {
		var err error
		data, err = blob.FetchToBytes(e.ctx, cursor)
		return err
	})
	if err != nil {
		e.t.Fatal(err)
	}
	typeID, err := world_types.GetObjectType(e.ctx, e.tb.WorldState, key)
	if err != nil {
		e.t.Fatal(err)
	}
	return string(data), typeID
}

// TestStandingPureFold feeds events with payloads through a keyed pure fold
// into a state object, with two run controllers racing as two Devices would.
// Each event is folded exactly once, the fold's output reaches a Target Step
// as a Task-owned value, and the run keeps running with nothing waiting.
func TestStandingPureFold(t *testing.T) {
	// Register the fold and create its graph.
	e := newEnv(t)
	fold := &foldFunction{barrier: make(chan struct{})}
	release, err := e.tb.Bus.AddController(e.ctx, &pureFunctions{id: "test/fold", fn: fold}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	e.foldGraph()
	key := e.standingRun()

	// Run two controllers of the same run, as two Devices do.
	e.startWorker()
	for range 2 {
		ctx, cancel := context.WithCancel(e.ctx)
		c := flowgraph_run.NewController(e.tb.Logger, e.tb.Bus, flowgraph_run.NewConfig(e.tb.EngineID, key))
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = c.Execute(ctx)
		}()
		t.Cleanup(func() {
			cancel()
			<-done
			_ = c.Close()
		})
	}

	// Deliver two events for one key; the first is evaluated by both.
	if result := e.deliverStanding("", "c1", payload("e1", "5")); result != flowgraph_run.DeliverEventsResult_DELIVER_EVENTS_RESULT_DELIVERED {
		t.Fatalf("first delivery = %s", result)
	}
	if result := e.deliverStanding("c1", "c2", payload("e2", "7")); result != flowgraph_run.DeliverEventsResult_DELIVER_EVENTS_RESULT_DELIVERED {
		t.Fatalf("second delivery = %s", result)
	}

	// Each event is folded once and its output runs Step a once.
	var run *s4wave_flowgraph.FlowgraphRun
	e.waitFor("fold and Step a visits", func() bool {
		run = e.readStanding()
		return run.GetVisits()["fold"].GetCount() == 2 && run.GetVisits()["a"].GetCount() == 2 && len(run.GetActivations()) == 0
	})
	if run.GetState() != s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING || len(run.GetArrivals()) != 0 {
		t.Fatalf("standing run = %v, want running with no arrivals", run)
	}
	fold.mtx.Lock()
	calls := fold.calls
	fold.mtx.Unlock()
	if calls < 3 {
		t.Fatalf("fold evaluations = %d, want both controllers on the first event and one more", calls)
	}
	taskKeys, err := forge_job.ListJobTasks(e.ctx, e.tb.WorldState, flowgraph_run.JobKey(key))
	if err != nil || len(taskKeys) != 2 {
		t.Fatalf("job tasks = %v, %v; want one per folded event", taskKeys, err)
	}
	if state, typeID := e.readState("fold/k"); state != "12" || typeID != "test/fold-state" {
		t.Fatalf("state = %q of type %q, want 12 of type test/fold-state", state, typeID)
	}
}

// foldGraph creates a graph of an event source feeding a keyed pure fold
// into a state object, whose output feeds a Target Step a.
func (e *env) foldGraph() {
	source := &s4wave_flowgraph.FlowgraphNode{
		TypeId: "test/event-source",
		Ports:  []*s4wave_flowgraph.FlowgraphPort{port("events", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT)},
	}
	pure := &s4wave_flowgraph.FlowgraphNode{
		TypeId: s4wave_flowgraph.StepNodeTypeID,
		Step: &s4wave_flowgraph.FlowgraphStep{Keyed: true, Exec: &s4wave_flowgraph.FlowgraphStep_Pure{Pure: &s4wave_flowgraph.PureStep{
			FunctionId:  "test/fold",
			StatePrefix: "fold/",
			StateTypeId: "test/fold-state",
		}}},
		Ports: []*s4wave_flowgraph.FlowgraphPort{
			port("in", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT),
			port("out", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT),
		},
	}
	e.createGraph(map[string]*s4wave_flowgraph.FlowgraphNode{
		sourceID: source,
		"fold":   pure,
		"a":      e.step([]string{"in"}, [2]string{"done", "none"}, true, nil),
	}, map[string]*s4wave_flowgraph.FlowgraphConnection{
		"src-fold": connect(sourceID, "events", "fold", "in"),
		"fold-a":   connect("fold", "out", "a", "in"),
	})
}

// payload builds an event for key k carrying a decimal payload.
func payload(id, value string) *flowgraph_run.DeliveredEvent {
	ev := event(id, "k")
	ev.Payload = []byte(value)
	return ev
}

// foldOnce evaluates the fold's ready inputs in ws and commits the result
// there, as one Device's pure watcher does, and reports whether it committed.
func (e *env) foldOnce(ctx context.Context, ws world.WorldState, eval *flowgraph_run.PureEval) (bool, error) {
	resp, err := (&foldFunction{calls: 2}).Evaluate(ctx, eval.Request())
	if err != nil {
		return false, err
	}
	return flowgraph_run.CommitPure(ctx, ws, standingKey, "fold", eval, resp)
}

// readFold reads the fold's ready evaluation from the committed World.
func (e *env) readFold() *flowgraph_run.PureEval {
	e.t.Helper()
	eval, err := flowgraph_run.ReadPure(e.ctx, e.tb.WorldState, standingKey, "fold")
	if err != nil || eval == nil {
		e.t.Fatalf("read fold: %v, %v", eval, err)
	}
	return eval
}

// recordable lets a test record a write transaction as a Space records it.
type recordable struct {
	world.Tx
}

// TakeSetRoots names no roots; the replay below needs none.
func (recordable) TakeSetRoots() []*block.BlockRef {
	return nil
}

// record runs fn in a write transaction on the current head and returns the
// operations it recorded, as a Device proposes them, without committing it.
func (e *env) record(fn func(ctx context.Context, ws world.WorldState) error) *world_block_tx.TxBatch {
	e.t.Helper()
	tx, err := e.tb.Engine.NewTransaction(e.ctx, true)
	if err != nil {
		e.t.Fatal(err)
	}
	defer tx.Discard()
	rec, err := world_block_tx.NewWorldState(e.ctx, recordable{tx}, true)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := fn(e.ctx, rec); err != nil {
		e.t.Fatal(err)
	}
	return rec.GetTxBatch().CloneVT()
}

// replay runs recorded operations on the current head and commits them, as a
// Space replays a Device's operation ordered after the head moved, and
// returns the error that refuses them.
func (e *env) replay(batch *world_block_tx.TxBatch) error {
	lookup := world.NewSchema(slices.Concat(world_expect.ExpectTypes, forge_world.Types)).LookupOp
	return world.ExecTransaction(e.ctx, e.tb.Engine, true, func(ctx context.Context, tx world.WorldState) error {
		_, err := batch.ExecuteTx(ctx, e.sender, lookup, tx)
		return err
	})
}

// TestReplayOnMovedRun replays two writes Device B recorded on a run that
// Device A has since moved, as a Space does for B's operations ordered after
// A's. A folded the event B also folded, started Step a from the fold's
// output, and delivered the event B also read. Each replay is refused, so no
// arrival is lost, the cursor does not go back, and Step a does not start a
// second time.
func TestReplayOnMovedRun(t *testing.T) {
	// Create the fold graph's standing run holding one event.
	e := newEnv(t)
	e.foldGraph()
	e.standingRun()
	if result := e.deliverStanding("", "c1", payload("e1", "5")); result != flowgraph_run.DeliverEventsResult_DELIVER_EVENTS_RESULT_DELIVERED {
		t.Fatalf("first delivery = %s", result)
	}

	// Device B delivers the next event and folds the first on this head.
	deliveryB := e.record(func(ctx context.Context, ws world.WorldState) error {
		_, err := flowgraph_run.DeliverEvents(ctx, ws, standingKey, &flowgraph_run.DeliverEventsRequest{
			NodeId: sourceID, Cursor: []byte("c1"), NextCursor: []byte("c2b"), Events: []*flowgraph_run.DeliveredEvent{payload("e2b", "9")},
		})
		return err
	})
	foldB := e.record(func(ctx context.Context, ws world.WorldState) error {
		_, err := e.foldOnce(ctx, ws, e.readFold())
		return err
	})

	// Device A folds the first event, starts Step a and delivers the next.
	eval := e.readFold()
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		_, err := e.foldOnce(ctx, tx, eval)
		return err
	})
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		return flowgraph_run.Advance(ctx, tx, standingKey)
	})
	if result := e.deliverStanding("c1", "c2", payload("e2", "7")); result != flowgraph_run.DeliverEventsResult_DELIVER_EVENTS_RESULT_DELIVERED {
		t.Fatalf("second delivery = %s", result)
	}
	before := e.readStanding()
	if len(before.GetActivations()) != 1 || before.GetVisits()["fold"].GetCount() != 1 {
		t.Fatalf("run after A's writes = %v, want Step a started once from one fold", before)
	}

	// Replaying either of B's writes on A's head is refused.
	for name, batch := range map[string]*world_block_tx.TxBatch{"delivery": deliveryB, "fold": foldB} {
		if err := e.replay(batch); !errors.Is(err, world_expect.ErrRootMoved) {
			t.Fatalf("replay of B's %s = %v, want %v", name, err, world_expect.ErrRootMoved)
		}
	}

	// The run keeps A's arrival, cursor and activation, and the state A's fold.
	after := e.readStanding()
	if !after.EqualVT(before) {
		t.Fatalf("replay changed the run: %v, was %v", after, before)
	}
	requireEventIDs(t, after, "e2")
	if got := string(after.GetCursor()[sourceID]); got != "c2" {
		t.Fatalf("cursor = %q, want c2", got)
	}
	if state, _ := e.readState("fold/k"); state != "5" {
		t.Fatalf("state = %q, want 5", state)
	}
}

// TestPureCommitStateMoved commits an evaluation whose arrival the run still
// holds in content, as a second equal event, after another evaluation moved
// the state it read. The stale evaluation must not commit, since it would
// fold onto the old state and lose the first fold.
func TestPureCommitStateMoved(t *testing.T) {
	// Create the fold graph's standing run holding two equal events.
	e := newEnv(t)
	e.foldGraph()
	e.standingRun()
	if result := e.deliverStanding("", "c1", payload("e1", "5"), payload("e1", "5")); result != flowgraph_run.DeliverEventsResult_DELIVER_EVENTS_RESULT_DELIVERED {
		t.Fatalf("delivery = %s", result)
	}

	// Read an evaluation, then fold the first event with a fresh one.
	stale := e.readFold()
	fresh := e.readFold()
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		_, err := e.foldOnce(ctx, tx, fresh)
		return err
	})

	// The stale evaluation matches the second event but read the old state.
	var committed bool
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		var err error
		committed, err = e.foldOnce(ctx, tx, stale)
		return err
	})
	if committed {
		t.Fatal("stale evaluation committed onto a moved state")
	}
	if state, _ := e.readState("fold/k"); state != "5" {
		t.Fatalf("state = %q, want 5", state)
	}

	// A fresh evaluation folds the second event onto the moved state.
	again := e.readFold()
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		_, err := e.foldOnce(ctx, tx, again)
		return err
	})
	if state, _ := e.readState("fold/k"); state != "10" {
		t.Fatalf("state = %q, want 10", state)
	}
}
