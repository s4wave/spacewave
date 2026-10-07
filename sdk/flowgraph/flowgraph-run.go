package s4wave_flowgraph

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

const (
	// FlowgraphRunTypeID is the World type of a Flowgraph run.
	FlowgraphRunTypeID = "flowgraph-run"
	// SpendOutputName is the reserved Step output reporting an activation's
	// spend as a decimal integer blob.
	SpendOutputName = "spend"
)

// NewFlowgraphRunBlock constructs the stored run state.
func NewFlowgraphRunBlock() block.Block {
	return &FlowgraphRun{}
}

// UnmarshalFlowgraphRun reads a run from a block cursor.
func UnmarshalFlowgraphRun(ctx context.Context, cursor *block.Cursor) (*FlowgraphRun, error) {
	return block.UnmarshalBlock[*FlowgraphRun](ctx, cursor, NewFlowgraphRunBlock)
}

// FollowGraph reads the pinned Flowgraph body. cursor points to the run.
func (r *FlowgraphRun) FollowGraph(ctx context.Context, cursor *block.Cursor) (*Flowgraph, error) {
	graph, err := UnmarshalFlowgraph(ctx, cursor.FollowRef(3, r.GetGraphRef()))
	if err != nil {
		return nil, err
	}
	if graph == nil {
		return nil, errors.New("flowgraph run has no pinned graph")
	}
	return graph, nil
}

// SetGraph pins a Flowgraph body and its revision. cursor points to the run;
// the caller writes the run block afterwards.
func (r *FlowgraphRun) SetGraph(cursor *block.Cursor, graph *Flowgraph, revision uint64) {
	// Write the graph as a new block; the run's reference is rebuilt on write.
	graphCursor := cursor.FollowRef(3, nil)
	graphCursor.ClearAllRefs()
	graphCursor.SetBlock(graph, true)
	r.GraphRef = nil
	r.FlowgraphRevision = revision
}

// MarshalBlock encodes the run state.
func (r *FlowgraphRun) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock decodes the run state.
func (r *FlowgraphRun) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// ApplyBlockRef applies the pinned graph reference.
func (r *FlowgraphRun) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	if id == 3 {
		r.GraphRef = ptr
	}
	return nil
}

// GetBlockRefs returns the pinned graph reference.
func (r *FlowgraphRun) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{3: r.GetGraphRef()}, nil
}

// GetBlockRefCtor returns the Flowgraph constructor for the pinned graph.
func (r *FlowgraphRun) GetBlockRefCtor(id uint32) block.Ctor {
	if id == 3 {
		return NewFlowgraphBlock
	}
	return nil
}

// _ is a type assertion.
var _ block.BlockWithRefs = (*FlowgraphRun)(nil)
