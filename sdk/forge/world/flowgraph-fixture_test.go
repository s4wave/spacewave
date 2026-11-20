//go:build linux && !tinygo

package s4wave_forge_world

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_task "github.com/s4wave/spacewave/forge/task"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_run "github.com/s4wave/spacewave/sdk/flowgraph/run"
)

// flowgraphFixture drives Flowgraph runs on a Worker harness and waits on
// their activations through World revisions.
type flowgraphFixture struct {
	*workerHarness
	t *testing.T
}

// transact commits one graph operation through the real World engine.
func (f *flowgraphFixture) transact(fn func(context.Context, world.WorldState) error) {
	if err := world.ExecTransaction(f.ctx, f.tb.Engine, true, fn); err != nil {
		f.t.Fatal(err)
	}
}

// wait waits on World revisions rather than polling runtime state.
func (f *flowgraphFixture) wait(what string, ready func() bool) {
	// Take the revision before observing its derived state, then wait for its change.
	for {
		seq, err := f.tb.WorldState.GetSeqno(f.ctx)
		if err != nil {
			f.t.Fatal(err)
		}
		if ready() {
			return
		}
		if _, err := f.tb.WorldState.WaitSeqno(f.ctx, seq+1); err != nil {
			f.t.Fatalf("waiting for %s: %v", what, err)
		}
	}
}

// readRun releases the object handle after decoding the bounded fixture run.
func (f *flowgraphFixture) readRun(key string) *s4wave_flowgraph.FlowgraphRun {
	// Decode only this run's state through its World object cursor.
	obj, err := world.MustGetObject(f.ctx, f.tb.WorldState, key)
	if err != nil {
		f.t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)
	var run *s4wave_flowgraph.FlowgraphRun
	_, _, err = world.AccessObjectState(f.ctx, obj, false, func(cursor *block.Cursor) error {
		var err error
		run, err = s4wave_flowgraph.UnmarshalFlowgraphRun(f.ctx, cursor)
		return err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return run
}

// waitTask waits for a graph activation and optionally its successful completion.
func (f *flowgraphFixture) waitTask(runKey, node string, complete bool) string {
	// Resolve the activation's Task, reporting a paused graph instead of waiting forever.
	var taskKey string
	f.wait(node, func() bool {
		run := f.readRun(runKey)
		if run.GetState() == s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_PAUSED {
			f.t.Fatalf("graph paused: %s", run.GetPauseReason())
		}
		for _, a := range run.GetActivations() {
			if a.GetNodeId() == node {
				taskKey = a.GetTaskKey()
				if !complete {
					return true
				}
				task, err := forge_task.LookupTaskBody(f.ctx, f.tb.WorldState, taskKey)
				if err != nil {
					f.t.Fatal(err)
				}
				if task.GetTaskState() == forge_task.State_TaskState_COMPLETE {
					if failure := task.GetResult().GetFailError(); failure != "" {
						f.t.Fatal(failure)
					}
					return true
				}
			}
		}
		return false
	})
	return taskKey
}

// startRun starts a distinct execution of an authored graph.
func (f *flowgraphFixture) startRun(key, graph string) {
	// Publish the run before attaching its existing controller factory.
	f.transact(func(ctx context.Context, ws world.WorldState) error {
		return flowgraph_run.StartRun(ctx, ws, f.sender, key, graph, harnessClusterKey, nil)
	})
	_, ref, err := flowgraph_run.StartControllerWithConfig(f.ctx, f.tb.Bus, flowgraph_run.NewConfig(f.tb.EngineID, key))
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(ref.Release)
}
