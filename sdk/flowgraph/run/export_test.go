package flowgraph_run

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	flowgraph_pure "github.com/s4wave/spacewave/sdk/flowgraph/pure"
)

// Advance advances the run in ws, as the run controller does.
func Advance(ctx context.Context, ws world.WorldState, runKey string) error {
	_, _, err := advance(ctx, ws, runKey)
	return err
}

// PureEval is a pure Step's evaluation read from a run.
type PureEval = pureEval

// Request returns the evaluation's request.
func (e *pureEval) Request() *flowgraph_pure.EvaluateRequest {
	return e.req
}

// ReadPure reads the pure Step's evaluation from the run in ws, or nil when
// the Step is not ready.
func ReadPure(ctx context.Context, ws world.WorldState, runKey, nodeID string) (*PureEval, error) {
	obj, run, graph, err := readRun(ctx, ws, runKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	r := &runner{runKey: runKey, ws: ws, graph: graph, run: run}
	return r.readPure(ctx, pureKey{nodeID: nodeID, functionID: graph.GetNodes()[nodeID].GetStep().GetPure().GetFunctionId()})
}

// CommitPure commits an evaluation's response in ws, as the pure watcher does.
func CommitPure(ctx context.Context, ws world.WorldState, runKey, nodeID string, eval *PureEval, resp *flowgraph_pure.EvaluateResponse) (bool, error) {
	key := pureKey{nodeID: nodeID, functionID: eval.req.GetFunctionId()}
	return commitPure(ctx, ws, runKey, key, eval, resp)
}
