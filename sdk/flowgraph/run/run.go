package flowgraph_run

import (
	"context"
	"maps"
	"slices"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// JobKey returns the key of a run's Forge Job.
func JobKey(runKey string) string {
	return runKey + "/job"
}

// StartRun creates a run of a Flowgraph's current revision. The run gets a new
// Job assigned to the Cluster, and each Step without connected inputs gets a
// start arrival. Call it in a write transaction; the run controller for runKey
// starts the Steps.
func StartRun(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	runKey, flowgraphKey, clusterKey string,
	placement *forge_worker.Placement,
) error {
	// Read the Flowgraph revision the run pins.
	snapshot, err := s4wave_flowgraph.ReadFlowgraph(ctx, ws, flowgraphKey)
	if err != nil {
		return err
	}
	graph := snapshot.GetState()

	// Create the empty Job and assign it to the Cluster.
	jobKey := JobKey(runKey)
	obj, _, err := forge_job.CreateJobWithTasks(ctx, ws, sender, jobKey, nil, "", placement, timestamp.Now())
	world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if _, _, err := forge_cluster.AssignJobToCluster(ctx, ws, clusterKey, jobKey, sender); err != nil {
		return err
	}

	// Queue a start arrival for each source Step.
	run := &s4wave_flowgraph.FlowgraphRun{
		FlowgraphKey: flowgraphKey,
		JobKey:       jobKey,
		State:        s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING,
	}
	for _, id := range slices.Sorted(maps.Keys(graph.GetNodes())) {
		if graph.GetNodes()[id].GetStep() != nil && len(inputPorts(graph, id)) == 0 {
			run.Arrivals = append(run.Arrivals, &s4wave_flowgraph.FlowgraphArrival{NodeId: id})
		}
	}

	// Store the run with its pinned graph.
	obj, _, err = world.CreateWorldObject(ctx, ws, runKey, func(cursor *block.Cursor) error {
		cursor.SetBlock(run, true)
		run.SetGraph(cursor, graph, snapshot.GetRevision())
		return nil
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	return world_types.SetObjectType(ctx, ws, runKey, s4wave_flowgraph.FlowgraphRunTypeID)
}

// ResumeRun continues a paused run on the Flowgraph's current revision: the
// pinned one when the owner has not edited the graph, or the newer one. The
// Step the run paused at gets fresh bound counters, and arrivals the current
// graph no longer accepts are dropped. Call it in a write transaction.
func ResumeRun(ctx context.Context, ws world.WorldState, runKey string) error {
	// Read the run; only a paused run resumes.
	obj, run, _, err := readRun(ctx, ws, runKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	switch run.GetState() {
	case s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_PAUSED:
	case s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING:
		return nil
	default:
		return errors.Errorf("cannot resume a run in state %s", run.GetState())
	}

	// Read the Flowgraph revision the run continues on.
	snapshot, err := s4wave_flowgraph.ReadFlowgraph(ctx, ws, run.GetFlowgraphKey())
	if err != nil {
		return err
	}
	graph := snapshot.GetState()

	// Reset the paused Step and keep the arrivals the graph still accepts.
	next := run.CloneVT()
	delete(next.Visits, next.GetPausedNode())
	next.State = s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING
	next.PausedNode, next.PauseReason = "", ""
	next.Arrivals = slices.DeleteFunc(next.Arrivals, func(arrival *s4wave_flowgraph.FlowgraphArrival) bool {
		return !accepts(graph, arrival)
	})

	// Write the run with the graph pinned at the current revision.
	_, _, err = world.AccessObjectState(ctx, obj, true, func(cursor *block.Cursor) error {
		cursor.SetBlock(next, true)
		next.SetGraph(cursor, graph, snapshot.GetRevision())
		return nil
	})
	return err
}

// accepts reports whether an arrival reaches a Step input of the graph.
func accepts(graph *s4wave_flowgraph.Flowgraph, arrival *s4wave_flowgraph.FlowgraphArrival) bool {
	if graph.GetNodes()[arrival.GetNodeId()].GetStep() == nil {
		return false
	}
	ports := inputPorts(graph, arrival.GetNodeId())
	if arrival.GetPort() == "" {
		return len(ports) == 0
	}
	return slices.Contains(ports, arrival.GetPort())
}
