package flowgraph_run_test

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_json "github.com/s4wave/spacewave/forge/target/json"
	forge_value "github.com/s4wave/spacewave/forge/value"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_run "github.com/s4wave/spacewave/sdk/flowgraph/run"
)

const (
	// liveStepID is the controller ID of the test Step that reads live inputs.
	liveStepID = "test/live-step"
	// keyedID is the keyed Step of the live tests.
	keyedID = "a"
	// templateKey names the test actor template.
	templateKey = "template/test"
)

// liveConfig is the test Step's config: the World engine and the run to read.
type liveConfig struct {
	*flowgraph_run.Config
}

// GetConfigID returns the test Step's config ID.
func (c *liveConfig) GetConfigID() string { return liveStepID }

// EqualsConfig checks if the other config is equal.
func (c *liveConfig) EqualsConfig(other config.Config) bool {
	oc, ok := other.(*liveConfig)
	return ok && c.EqualVT(oc.Config)
}

// liveSteps is the test Step factory. Each Step execution takes one token
// before it completes, so a test holds activations running. A Step with ack
// set reads its live inputs the way a code Step does: it echoes the event ID
// of each and acknowledges it. A Step without ack reads none.
type liveSteps struct {
	e *env
	// ack makes each execution echo and acknowledge its live inputs.
	ack bool
	// tokens releases one execution per token.
	tokens chan struct{}
	// echoed receives the event ID of each live input an execution read.
	echoed chan string
	// runKey is the run the executions belong to.
	runKey string
	// attached reports each actor an execution attaches to.
	attached chan string
	// stopped reports executions whose lifecycle ended.
	stopped chan string
	// stopWorker stops the current worker and its executions.
	stopWorker func()
	// outputKey overrides the routing key on completed activations when nonempty.
	outputKey string
}

// GetConfigID returns the config ID.
func (f *liveSteps) GetConfigID() string { return liveStepID }

// GetControllerID returns the controller ID.
func (f *liveSteps) GetControllerID() string { return liveStepID }

// ConstructConfig constructs an empty config.
func (f *liveSteps) ConstructConfig() config.Config {
	return &liveConfig{Config: &flowgraph_run.Config{}}
}

// Construct constructs a Step execution.
func (f *liveSteps) Construct(context.Context, config.Config, controller.ConstructOpts) (controller.Controller, error) {
	return &liveStep{f: f}, nil
}

// GetVersion returns the version.
func (f *liveSteps) GetVersion() controller.Version { return flowgraph_run.Version }

// liveStep is one execution of the test Step.
type liveStep struct {
	f      *liveSteps
	handle forge_target.ExecControllerHandle
	// activation is the context supplied on the production Task input.
	activation *s4wave_flowgraph.FlowgraphActivationContext
}

// GetControllerInfo returns information about the controller.
func (s *liveStep) GetControllerInfo() *controller.Info {
	return controller.NewInfo(liveStepID, flowgraph_run.Version, "test Step that reads live inputs")
}

// InitForgeExecController keeps the handle.
func (s *liveStep) InitForgeExecController(ctx context.Context, inputs forge_target.InputMap, handle forge_target.ExecControllerHandle) error {
	// Keep the handle and read this activation's own identity.
	s.handle = handle
	var err error
	s.activation, err = flowgraph_run.ReadActivationContext(ctx, inputs, handle)
	return err
}

// attach finds the recorded actor, or creates and records one atomically.
func (s *liveStep) attach(ctx context.Context) (string, error) {
	// Read the run through the same transaction that will record the actor.
	var actor string
	err := world.ExecTransaction(ctx, s.f.e.tb.Engine, true, func(ctx context.Context, tx world.WorldState) error {
		run, _, err := s4wave_flowgraph.ReadFlowgraphRun(ctx, tx, s.activation.GetRunKey())
		if err != nil {
			return err
		}
		// Reattach only while the owning activation still exists.
		found := false
		for _, activation := range run.GetActivations() {
			if activation.GetTaskKey() == s.activation.GetActivationId() {
				actor = activation.GetActorKey()
				found = true
				break
			}
		}
		if !found {
			return flowgraph_run.ErrActivationEnded
		}
		if actor != "" {
			return nil
		}

		// A template belongs to the pinned placement, even if the graph was edited.
		if s.activation.GetTemplateKey() != templateKey {
			return errors.New("wrong activation template")
		}
		actor = "test-actor/" + s.activation.GetActivationId()
		obj, _, err := world.CreateWorldObject(ctx, tx, actor, func(cursor *block.Cursor) error {
			cursor.SetBlock(s.activation, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		return flowgraph_run.RecordActivationActor(ctx, tx, s.activation.GetRunKey(), s.activation.GetActivationId(), actor)
	})
	return actor, err
}

// Execute reads live inputs while it waits for a token, then sets its output.
func (s *liveStep) Execute(ctx context.Context) error {
	// Start one actor for this activation, or reattach after a worker restart.
	actor, err := s.attach(ctx)
	if err != nil {
		return err
	}
	s.f.attached <- actor
	defer func() { s.f.stopped <- actor }()

	// Echo and acknowledge each live input until the token arrives.
	e := s.f.e
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	watched := make(chan error, 1)
	if s.f.ack {
		id := s.activation.GetActivationId()
		go func() {
			watched <- flowgraph_run.WatchLiveInputs(watchCtx, e.tb.WorldState, s.f.runKey, id, func(inputs []*s4wave_flowgraph.FlowgraphLiveInput) error {
				for _, input := range inputs {
					select {
					case s.f.echoed <- input.GetArrival().GetEvent().GetEventId():
					case <-watchCtx.Done():
						return watchCtx.Err()
					}
				}
				last := inputs[len(inputs)-1].GetSeq()
				return world.ExecTransaction(watchCtx, e.tb.Engine, true, func(ctx context.Context, tx world.WorldState) error {
					return flowgraph_run.AckLiveInputs(ctx, tx, s.f.runKey, id, last)
				})
			})
		}()
	}
	select {
	case <-s.f.tokens:
	case <-ctx.Done():
		return ctx.Err()
	}
	cancel()
	if s.f.ack {
		if err := <-watched; !errors.Is(err, context.Canceled) {
			return err
		}
	}

	// Choose the first output port.
	seed := forge_value.NewValueWithBlockRef("", nil)
	val, err := forge_target.AccessValue(ctx, s.handle, seed, func(bcs *block.Cursor) error {
		_, err := blob.BuildBlob(ctx, 2, bytes.NewReader([]byte("ok")), bcs, nil)
		return err
	})
	if err != nil {
		return err
	}
	val.Name = "out"
	outputs := forge_value.ValueSlice{val}
	if s.f.outputKey != "" {
		keyValue, err := forge_target.AccessValue(ctx, s.handle, seed, func(cursor *block.Cursor) error {
			_, err := blob.BuildBlob(ctx, int64(len(s.f.outputKey)), bytes.NewBufferString(s.f.outputKey), cursor, nil)
			return err
		})
		if err != nil {
			return err
		}
		keyValue.Name = s4wave_flowgraph.KeyOutputName
		outputs = append(outputs, keyValue)
	}
	return s.handle.SetOutputs(ctx, outputs, true)
}

// HandleDirective resolves no directives.
func (s *liveStep) HandleDirective(context.Context, directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases nothing.
func (s *liveStep) Close() error { return nil }

// _ is a type assertion
var _ forge_target.ExecController = (*liveStep)(nil)

// liveGraph creates a Flowgraph of the event sources feeding one keyed Step
// that runs the test controller, with the standing run, its Worker and its
// controller started, and returns the test Step factory. The Step has one
// input port per source.
func (e *env) liveGraph(ack bool, sources ...string) *liveSteps {
	return e.liveGraphWith(ack, false, sources...)
}

// liveGraphWith optionally connects a keyed successor to the test template Step.
func (e *env) liveGraphWith(ack, successor bool, sources ...string) *liveSteps {
	// Register the test Step.
	steps := &liveSteps{e: e, ack: ack, tokens: make(chan struct{}, 8), echoed: make(chan string, 8), runKey: standingKey, attached: make(chan string, 16), stopped: make(chan string, 16)}
	e.tb.StaticResolver.AddFactory(steps)
	target, err := target_json.ResolveYAML(e.ctx, e.tb.Bus, []byte(`
outputs:
  - name: out
    outputType: OutputType_EXEC
    execOutput: out
  - name: none
    outputType: OutputType_EXEC
    execOutput: none
  - name: flowgraph/key
    outputType: OutputType_EXEC
    execOutput: flowgraph/key
exec:
  controller:
    id: `+liveStepID+`
    config:
      engineId: `+strconv.Quote(e.tb.EngineID)+`
      objectKey: `+strconv.Quote(standingKey)+`
`))
	if err != nil {
		e.t.Fatal(err)
	}

	// Connect each source to its own input port of the keyed Step.
	step := &s4wave_flowgraph.FlowgraphNode{
		TypeId: s4wave_flowgraph.StepNodeTypeID,
		Step:   &s4wave_flowgraph.FlowgraphStep{Target: target, Keyed: true},
		Ports: []*s4wave_flowgraph.FlowgraphPort{
			port("out", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT),
			port("none", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT),
		},
	}
	nodes := map[string]*s4wave_flowgraph.FlowgraphNode{keyedID: step}
	connections := make(map[string]*s4wave_flowgraph.FlowgraphConnection)
	for _, source := range sources {
		nodes[source] = &s4wave_flowgraph.FlowgraphNode{
			TypeId: "test/event-source",
			Ports:  []*s4wave_flowgraph.FlowgraphPort{port("events", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT)},
		}
		step.Ports = append(step.Ports, port("in-"+source, s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT))
		connections[source+"-"+keyedID] = connect(source, "events", keyedID, "in-"+source)
	}
	if successor {
		nodes["b"] = step.CloneVT()
		nodes["b"].Ports = append(nodes["b"].Ports[:2], port("in", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT))
		connections["a-b"] = connect(keyedID, "out", "b", "in")
	}
	e.createGraph(nodes, connections)

	// Create the event object, the standing run, and start both controllers.
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, tx, eventObjectKey, func(cursor *block.Cursor) error {
			cursor.SetBlock(&s4wave_flowgraph.FlowgraphVisits{}, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		return err
	})
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, tx, templateKey, func(cursor *block.Cursor) error {
			cursor.SetBlock(&s4wave_flowgraph.FlowgraphVisits{}, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		return err
	})
	e.editGraph(&s4wave_flowgraph.UpdateFlowgraphRequest{SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacements{
		keyedID: {Copies: []*s4wave_flowgraph.FlowgraphPlacement{{Kind: s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR_TEMPLATE, ObjectKey: templateKey}}},
	}})
	if successor {
		e.editGraph(&s4wave_flowgraph.UpdateFlowgraphRequest{SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacements{
			"b": {Copies: []*s4wave_flowgraph.FlowgraphPlacement{{Kind: s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR_TEMPLATE, ObjectKey: templateKey}}},
		}})
	}
	e.standingRun()
	steps.stopWorker = e.startWorker()
	_, ref, err := flowgraph_run.StartControllerWithConfig(e.ctx, e.tb.Bus, flowgraph_run.NewConfig(e.tb.EngineID, standingKey))
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(ref.Release)
	return steps
}

// deliverFrom delivers events from a source node to the standing run.
func (e *env) deliverFrom(source, cursor, next string, events ...*flowgraph_run.DeliveredEvent) {
	e.t.Helper()
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		result, err := flowgraph_run.DeliverEvents(ctx, tx, standingKey, &flowgraph_run.DeliverEventsRequest{
			NodeId:     source,
			Cursor:     []byte(cursor),
			NextCursor: []byte(next),
			Events:     events,
		})
		if err == nil && result != flowgraph_run.DeliverEventsResult_DELIVER_EVENTS_RESULT_DELIVERED {
			e.t.Fatalf("delivery from %s = %s", source, result)
		}
		return err
	})
}

// waitStandingRun waits until the standing run satisfies done and returns it.
func (e *env) waitStandingRun(what string, done func(*s4wave_flowgraph.FlowgraphRun) bool) *s4wave_flowgraph.FlowgraphRun {
	e.t.Helper()
	var run *s4wave_flowgraph.FlowgraphRun
	e.waitFor(what, func() bool {
		run = e.readStanding()
		return done(run)
	})
	return run
}

// release lets n Step executions complete.
func (f *liveSteps) release(n int) {
	for range n {
		f.tokens <- struct{}{}
	}
}

// nextEcho waits for the next event ID a Step read live.
func (f *liveSteps) nextEcho() string {
	f.e.t.Helper()
	select {
	case id := <-f.echoed:
		return id
	case <-time.After(30 * time.Second):
		f.e.t.Fatal("timed out waiting for an echoed live input")
		return ""
	}
}

// eventID returns an arrival's event ID.
func eventID(arrival *s4wave_flowgraph.FlowgraphArrival) string {
	return arrival.GetEvent().GetEventId()
}

// requireDrained waits for the standing run to hold nothing and have started
// the Step the given number of times.
func (e *env) requireDrained(visits uint32) *s4wave_flowgraph.FlowgraphRun {
	e.t.Helper()
	run := e.waitStandingRun("drained run", func(run *s4wave_flowgraph.FlowgraphRun) bool {
		return run.GetVisits()[keyedID].GetCount() == visits && len(run.GetActivations()) == 0
	})
	if len(run.GetArrivals()) != 0 {
		e.t.Fatalf("drained run holds arrivals %v", eventIDs(run))
	}
	return run
}

// TestKeyedLiveDelivery sends a second arrival for a key while its activation
// runs, and reads it through the activation's inbox.
func TestKeyedLiveDelivery(t *testing.T) {
	// Start a keyed Step whose executions echo and acknowledge live inputs.
	e := newEnv(t)
	steps := e.liveGraph(true, sourceID)

	// The first arrival for key k starts an activation that records the key.
	e.deliverFrom(sourceID, "", "c1", event("e1", "k"))
	run := e.waitStandingRun("first activation", func(run *s4wave_flowgraph.FlowgraphRun) bool {
		return len(run.GetActivations()) == 1
	})
	first := run.GetActivations()[0]
	if first.GetKey() != "k" || len(first.GetInputs()) != 1 || eventID(first.GetInputs()[0]) != "e1" {
		t.Fatalf("first activation = %v, want key k with input e1", first)
	}

	// A second arrival for k reaches the running activation, which echoes it.
	e.deliverFrom(sourceID, "c1", "c2", event("e2", "k"))
	if got := steps.nextEcho(); got != "e2" {
		t.Fatalf("echoed %q, want e2", got)
	}

	// The acknowledgement empties the inbox, and no second activation started.
	run = e.waitStandingRun("acknowledged inbox", func(run *s4wave_flowgraph.FlowgraphRun) bool {
		return len(run.GetActivations()) == 1 && len(run.GetActivations()[0].GetInbox()) == 0 && len(run.GetArrivals()) == 0
	})
	if got := run.GetActivations()[0].GetLastInputSeq(); got != 1 {
		t.Fatalf("last input seq = %d, want 1", got)
	}
	if got := run.GetVisits()[keyedID].GetCount(); got != 1 {
		t.Fatalf("visits = %d, want 1", got)
	}

	// The activation ends without a successor, since it acknowledged e2.
	steps.release(1)
	e.requireDrained(1)
}

// TestKeyedActivationsRunTogether runs one activation per key at the same
// time and routes a repeat arrival to the activation of its key.
func TestKeyedActivationsRunTogether(t *testing.T) {
	// Start a keyed Step whose executions wait for a token.
	e := newEnv(t)
	steps := e.liveGraph(false, sourceID)

	// Arrivals for two keys run two activations at once.
	e.deliverFrom(sourceID, "", "c1", event("e1", "k1"), event("e2", "k2"))
	run := e.waitStandingRun("two activations", func(run *s4wave_flowgraph.FlowgraphRun) bool {
		return len(run.GetActivations()) == 2
	})
	keys := []string{run.GetActivations()[0].GetKey(), run.GetActivations()[1].GetKey()}
	if keys[0] != "k1" || keys[1] != "k2" || run.GetActivations()[0].GetDone() || run.GetActivations()[1].GetDone() {
		t.Fatalf("activations = %v, want k1 and k2 both running", run.GetActivations())
	}

	// Both Tasks receive their own context and name distinct actors on the run.
	actors := []string{<-steps.attached, <-steps.attached}
	if actors[0] == actors[1] {
		t.Fatal("two keys share an actor")
	}
	run = e.waitStandingRun("recorded actors", func(run *s4wave_flowgraph.FlowgraphRun) bool {
		return len(run.GetActivations()) == 2 && run.GetActivations()[0].GetActorKey() != "" && run.GetActivations()[1].GetActorKey() != ""
	})
	first := run.GetActivations()[0]
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		if err := flowgraph_run.RecordActivationActor(ctx, tx, standingKey, first.GetTaskKey(), first.GetActorKey()); err != nil {
			return err
		}
		if err := flowgraph_run.RecordActivationActor(ctx, tx, standingKey, first.GetTaskKey(), run.GetActivations()[1].GetActorKey()); err == nil {
			t.Fatal("actor replacement succeeded")
		}
		return nil
	})

	// Restart the Worker mid-activation and reattach to both recorded actors.
	steps.stopWorker()
	for range 2 {
		select {
		case <-steps.stopped:
		case <-e.ctx.Done():
			t.Fatal(e.ctx.Err())
		}
	}
	steps.stopWorker = e.startWorker()
	for range 2 {
		select {
		case actor := <-steps.attached:
			if actor != actors[0] && actor != actors[1] {
				t.Fatalf("restart created %q", actor)
			}
		case <-e.ctx.Done():
			t.Fatal(e.ctx.Err())
		}
	}

	// A repeat arrival for k1 joins k1's inbox and starts no third activation.
	e.deliverFrom(sourceID, "c1", "c2", event("e3", "k1"))
	run = e.waitStandingRun("k1 inbox", func(run *s4wave_flowgraph.FlowgraphRun) bool {
		return len(run.GetActivations()) == 2 && len(run.GetActivations()[0].GetInbox()) == 1
	})
	input := run.GetActivations()[0].GetInbox()[0]
	if input.GetSeq() != 1 || eventID(input.GetArrival()) != "e3" || input.GetArrival().GetKey() != "k1" || len(run.GetActivations()[1].GetInbox()) != 0 {
		t.Fatalf("activations = %v, want e3 in k1's inbox only", run.GetActivations())
	}
	if len(run.GetArrivals()) != 0 || run.GetVisits()[keyedID].GetCount() != 2 {
		t.Fatalf("run = %v, want no arrivals and two visits", run)
	}

	// Both executions end; k1's unacknowledged e3 starts a third activation.
	steps.release(2)
	successorActor := <-steps.attached
	if successorActor == actors[0] || successorActor == actors[1] {
		t.Fatal("successor reused its predecessor actor")
	}
	steps.release(1)
	e.requireDrained(3)
}

// TestKeyedSuccessorReceivesUnacknowledgedInput starts a successor with an
// input its predecessor never acknowledged.
func TestKeyedSuccessorReceivesUnacknowledgedInput(t *testing.T) {
	// Start a keyed Step whose executions read no live inputs.
	e := newEnv(t)
	steps := e.liveGraph(false, sourceID)

	// Deliver a second arrival for k while the first activation runs.
	e.deliverFrom(sourceID, "", "c1", event("e1", "k"))
	e.waitStandingRun("first activation", func(run *s4wave_flowgraph.FlowgraphRun) bool {
		return len(run.GetActivations()) == 1
	})
	e.deliverFrom(sourceID, "c1", "c2", event("e2", "k"))
	e.waitStandingRun("inbox", func(run *s4wave_flowgraph.FlowgraphRun) bool {
		return len(run.GetActivations()) == 1 && len(run.GetActivations()[0].GetInbox()) == 1
	})

	// The first activation ends, and its successor starts with e2 as its input.
	steps.release(1)
	run := e.waitStandingRun("successor", func(run *s4wave_flowgraph.FlowgraphRun) bool {
		return run.GetVisits()[keyedID].GetCount() == 2 && len(run.GetActivations()) == 1
	})
	successor := run.GetActivations()[0]
	if successor.GetKey() != "k" || successor.GetDone() || len(successor.GetInbox()) != 0 ||
		len(successor.GetInputs()) != 1 || eventID(successor.GetInputs()[0]) != "e2" || successor.GetInputs()[0].GetKey() != "k" {
		t.Fatalf("successor = %v, want a running activation for k with input e2", successor)
	}
	if len(run.GetArrivals()) != 0 {
		t.Fatalf("run holds arrivals %v, want none", eventIDs(run))
	}

	// The successor ends without another one.
	steps.release(1)
	e.requireDrained(2)
}

// TestKeyedOutputRouting carries a key through a Step and honors its reserved override.
func TestKeyedOutputRouting(t *testing.T) {
	for _, override := range []string{"", "topic-reassigned"} {
		t.Run("override="+override, func(t *testing.T) {
			// Connect two keyed template Steps and control their completion independently.
			e := newEnv(t)
			steps := e.liveGraphWith(false, true, sourceID)
			steps.outputKey = override
			e.deliverFrom(sourceID, "", "c1", event("message", "topic"))
			select {
			case <-steps.attached:
			case <-e.ctx.Done():
				t.Fatal(e.ctx.Err())
			}
			steps.release(1)

			// The successor receives either the inherited key or the explicit override.
			want := "topic"
			if override != "" {
				want = override
			}
			run := e.waitStandingRun("keyed successor", func(run *s4wave_flowgraph.FlowgraphRun) bool {
				return len(run.GetActivations()) == 1 && run.GetActivations()[0].GetNodeId() == "b"
			})
			activation := run.GetActivations()[0]
			if activation.GetKey() != want || activation.GetInputs()[0].GetKey() != want {
				t.Fatalf("successor = %v, want key %q", activation, want)
			}

			// Complete the successor and wait until both Steps have drained.
			select {
			case <-steps.attached:
			case <-e.ctx.Done():
				t.Fatal(e.ctx.Err())
			}
			steps.release(1)
			e.requireDrained(1)
		})
	}
}
