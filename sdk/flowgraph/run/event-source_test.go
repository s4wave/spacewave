package flowgraph_run_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_task "github.com/s4wave/spacewave/forge/task"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_run "github.com/s4wave/spacewave/sdk/flowgraph/run"
)

// counterKey is the object the Flowgraph's event-source node binds.
const counterKey = "events/counter"

// counterGraph registers the counter object type on the bus, creates the
// counter object, and creates a Flowgraph whose event-source node is bound to
// it and feeds one Step. It returns the source and the key of the graph's
// standing run, which it creates.
func (e *env) counterGraph() (*counterSource, string) {
	// Register the counter type the way a plugin does.
	src := &counterSource{}
	release, err := e.tb.Bus.AddController(e.ctx, &counterTypes{source: src}, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(release)

	// Create the graph and the counter object it binds.
	source := &s4wave_flowgraph.FlowgraphNode{
		TypeId:    s4wave_flowgraph.EventSourceNodeTypeID,
		ObjectKey: counterKey,
		Ports:     []*s4wave_flowgraph.FlowgraphPort{port("event", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT)},
	}
	e.createGraph(map[string]*s4wave_flowgraph.FlowgraphNode{
		sourceID: source,
		"a":      e.step([]string{"in"}, [2]string{"done", "none"}, true, nil),
	}, map[string]*s4wave_flowgraph.FlowgraphConnection{
		"src-a": connect(sourceID, "event", "a", "in"),
	})
	e.createCounter()
	return src, e.standingRun()
}

// createCounter creates the counter object with its type.
func (e *env) createCounter() {
	e.exec(func(ctx context.Context, tx world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, tx, counterKey, func(cursor *block.Cursor) error {
			cursor.SetBlock(&s4wave_flowgraph.FlowgraphVisits{}, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		return world_types.SetObjectType(ctx, tx, counterKey, counterType)
	})
}

// startControllerOn starts a run controller on a World engine and returns its
// stop function.
func (e *env) startControllerOn(engineID, key string) func() {
	_, ref, err := flowgraph_run.StartControllerWithConfig(e.ctx, e.tb.Bus, flowgraph_run.NewConfig(engineID, key))
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(ref.Release)
	return ref.Release
}

// waitWatching waits until the source has n open streams.
func (e *env) waitWatching(src *counterSource, n int) {
	e.t.Helper()
	if err := src.waitWatching(e.ctx, n); err != nil {
		e.t.Fatalf("waiting for %d open streams: %v", n, err)
	}
}

// waitVisits waits until Step a has run n times and the standing run holds no
// activation, and returns the run.
func (e *env) waitVisits(n uint32) *s4wave_flowgraph.FlowgraphRun {
	e.t.Helper()
	var run *s4wave_flowgraph.FlowgraphRun
	e.waitFor("Step a visits", func() bool {
		run = e.readStanding()
		return run.GetVisits()["a"].GetCount() == n && len(run.GetActivations()) == 0
	})
	return run
}

// requireDelivered checks that the standing run took exactly n events: Step a
// ran n times, the Job holds n complete Tasks, the run holds no activation and
// no arrival, it is still running, and the node's cursor is n.
func (e *env) requireDelivered(n int) {
	e.t.Helper()

	// Wait for Step a, then check the run's state and cursor.
	run := e.waitVisits(uint32(n))
	if run.GetState() != s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING || len(run.GetArrivals()) != 0 {
		e.t.Fatalf("standing run = %v, want running with no arrivals", run)
	}
	if got := string(run.GetCursor()[sourceID]); got != strconv.Itoa(n) {
		e.t.Fatalf("cursor = %q, want %d", got, n)
	}

	// Check the Job holds one complete Task for each event.
	taskKeys, err := forge_job.ListJobTasks(e.ctx, e.tb.WorldState, flowgraph_run.JobKey(standingKey))
	if err != nil {
		e.t.Fatal(err)
	}
	if len(taskKeys) != n {
		e.t.Fatalf("job tasks = %v, want %d", taskKeys, n)
	}
	for _, taskKey := range taskKeys {
		task, err := forge_task.LookupTaskBody(e.ctx, e.tb.WorldState, taskKey)
		if err != nil {
			e.t.Fatal(err)
		}
		if !task.IsComplete() {
			e.t.Fatalf("task %s is not complete", taskKey)
		}
	}
}

// TestEventSourceRestart stops the controller, emits an event while it is
// stopped, and starts it again. The event is delivered once, the events
// before it are not delivered again, and the source is acked through it.
func TestEventSourceRestart(t *testing.T) {
	// Deliver an event through a running controller.
	e := newEnv(t)
	src, key := e.counterGraph()
	e.startWorker()
	stop := e.startControllerOn(e.tb.EngineID, key)
	e.waitWatching(src, 1)
	src.emit(1)
	e.requireDelivered(1)

	// Stop the controller, then emit events it never reads.
	stop()
	e.waitWatching(src, 0)
	src.emit(2)

	// A restarted controller resumes from the stored cursor and acks it.
	e.startControllerOn(e.tb.EngineID, key)
	e.requireDelivered(3)
	if err := src.waitAcked(e.ctx, 3); err != nil {
		t.Fatalf("waiting for the ack of event 3: %v", err)
	}
}
