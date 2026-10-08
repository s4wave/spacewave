package flowgraph_run_test

import (
	"context"
	"testing"

	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	"github.com/s4wave/spacewave/db/volume"
	forge_task "github.com/s4wave/spacewave/forge/task"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
)

// TestBindingRecreationContinuesRun routes a completed Task after the Space
// process controller is recreated from the same approved local binding.
func TestBindingRecreationContinuesRun(t *testing.T) {
	// Create a two-Step run with no Worker yet, so its first Task stays pending.
	e := newEnv(t)
	e.createGraph(map[string]*s4wave_flowgraph.FlowgraphNode{
		"a": e.step(nil, [2]string{"yes", "no"}, true, nil),
		"b": e.step([]string{"in"}, [2]string{"done", "other"}, true, nil),
	}, map[string]*s4wave_flowgraph.FlowgraphConnection{
		"next": connect("a", "yes", "b", "in"),
	})
	e.startRun()
	e.tb.StaticResolver.AddFactory(plugin_space.NewFactory(e.tb.Bus))

	// Open the same account store the Space process reads.
	conf := &plugin_space.Config{
		SpaceId: "binding-test", VolumeId: e.tb.EngineVolumeID,
		ObjectStoreId: e.tb.EngineObjectStoreID, EngineId: e.tb.EngineID,
		SessionPeerId: e.sender.String(),
	}
	handle, _, storeRef, err := volume.ExBuildObjectStoreAPI(e.ctx, e.tb.Bus, true, conf.GetObjectStoreId(), conf.GetVolumeId(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer storeRef.Release()

	// Save the approval before starting the Space process controller.
	if err := process_binding.SetProcessBinding(e.ctx, handle.GetObjectStore(), conf.GetSpaceId(), runKey, &s4wave_process.ProcessBinding{
		ObjectKey: runKey, TypeId: s4wave_flowgraph.FlowgraphRunTypeID,
		State: s4wave_process.ProcessBindingState_ProcessBindingState_APPROVED,
	}); err != nil {
		t.Fatal(err)
	}

	// Start through the binding and wait for its first durable activation.
	firstCtx, cancelFirst := context.WithCancel(e.ctx)
	_, _, firstRef, err := plugin_space.StartControllerWithConfig(firstCtx, e.tb.Bus, conf, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(firstRef.Release)
	t.Cleanup(cancelFirst)
	e.waitFor("first bound activation", func() bool {
		return len(e.readRun().GetActivations()) == 1
	})

	// Stop the first binding generation without replacing its activation.
	firstTask := e.readRun().GetActivations()[0].GetTaskKey()
	cancelFirst()
	firstRef.Release()

	// Complete the pending Task while the run's first process is stopped.
	e.startWorker()
	e.waitFor("first Task complete", func() bool {
		task, err := forge_task.LookupTaskBody(e.ctx, e.tb.WorldState, firstTask)
		if err != nil {
			t.Fatal(err)
		}
		return task.IsComplete()
	})

	// Recreate the Space process without changing or reapproving the binding.
	_, _, secondRef, err := plugin_space.StartControllerWithConfig(e.ctx, e.tb.Bus, conf, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondRef.Release)
	run := e.waitRun(s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_COMPLETE)
	requireTrace(t, run, "a.yes", "b.done")
	e.requireInput(run, 1, "in", 0)
	if run.GetActivations()[0].GetTaskKey() != firstTask {
		t.Fatal("recreated binding replaced the first activation")
	}
}
