package s4wave_flowgraph

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// ReadFlowgraph reads a graph's body, placement edges, and object revision.
// Call with a transaction to keep the body and edges in one snapshot.
func ReadFlowgraph(ctx context.Context, ws world.WorldState, key string) (*FlowgraphSnapshot, error) {
	// Retain the object while decoding its body and revision.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}
	state, err := s4wave_world.ReadWorldBlock[*Flowgraph](ctx, obj, NewFlowgraphBlock)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &Flowgraph{}
	}

	// Read the placement edges and the revision from the same World state.
	placements, err := ReadPlacements(ctx, ws, key)
	if err != nil {
		return nil, err
	}
	_, revision, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, err
	}
	return &FlowgraphSnapshot{State: state, Placements: placements, Revision: revision}, nil
}

// UpdateFlowgraph changes the body and placement graph in the caller's transaction.
// The caller commits on success or discards the transaction on error.
func UpdateFlowgraph(ctx context.Context, ws world.WorldState, key string, request *UpdateFlowgraphRequest) (*FlowgraphSnapshot, error) {
	// Read the latest authored state from the write transaction.
	previous, err := ReadFlowgraph(ctx, ws, key)
	if err != nil {
		return nil, err
	}
	next := previous.CloneVT()
	state := next.GetState()
	if state.Nodes == nil {
		state.Nodes = make(map[string]*FlowgraphNode)
	}
	if state.Connections == nil {
		state.Connections = make(map[string]*FlowgraphConnection)
	}
	if next.Placements == nil {
		next.Placements = make(map[string]*FlowgraphPlacement)
	}
	if request.Name != nil {
		state.Name = request.GetName()
	}

	// Apply node edits and remove every connection incident to a deleted node.
	for id, node := range request.GetSetNodes() {
		state.Nodes[id] = node.CloneVT()
	}
	for _, id := range request.GetRemoveNodeIds() {
		delete(state.Nodes, id)
		delete(next.Placements, id)
		for connectionID, connection := range state.Connections {
			if connection.GetInputNode() == id || connection.GetOutputNode() == id {
				delete(state.Connections, connectionID)
			}
		}
	}

	// Apply connection edits before validating the complete resulting graph.
	for id, connection := range request.GetSetConnections() {
		state.Connections[id] = connection.CloneVT()
	}
	for _, id := range request.GetRemoveConnectionIds() {
		delete(state.Connections, id)
	}
	if err := state.Validate(); err != nil {
		return nil, err
	}

	// Apply placement edits and validate all remaining placements against the nodes.
	for id, placement := range request.GetSetPlacements() {
		next.Placements[id] = placement.CloneVT()
	}
	for _, id := range request.GetRemovePlacementNodeIds() {
		delete(next.Placements, id)
	}
	for id, placement := range next.Placements {
		node, found := state.Nodes[id]
		if !found {
			return nil, errors.Errorf("placement refers to missing node %q", id)
		}
		if err := ValidatePlacement(ctx, ws, node, placement); err != nil {
			return nil, errors.Wrapf(err, "node %q", id)
		}
	}

	// Remove replaced or deleted placement edges.
	var placed bool
	for id, placement := range previous.GetPlacements() {
		if placement.EqualVT(next.Placements[id]) {
			continue
		}
		edge, err := PlacementQuad(key, id, placement)
		if err != nil {
			return nil, err
		}
		if err := ws.DeleteGraphQuad(ctx, edge); err != nil {
			return nil, err
		}
		placed = true
	}

	// Add new placement edges in the same transaction as the authored body.
	for id, placement := range next.GetPlacements() {
		if placement.EqualVT(previous.Placements[id]) {
			continue
		}
		edge, err := PlacementQuad(key, id, placement)
		if err != nil {
			return nil, err
		}
		if err := ws.SetGraphQuad(ctx, edge); err != nil {
			return nil, err
		}
		placed = true
	}

	// Write the object last, so its World change closes this edit in the
	// changelog. A placement-only edit leaves the body as it was, so it marks
	// the object with a revision change instead.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}
	switch {
	case !state.EqualVT(previous.GetState()):
		_, _, err = world.AccessObjectState(ctx, obj, true, func(cursor *block.Cursor) error {
			cursor.SetBlock(state, true)
			return nil
		})
	case placed:
		_, err = obj.IncrementRev(ctx)
	}
	if err != nil {
		return nil, err
	}

	// Read back the transaction-local revision.
	_, next.Revision, err = obj.GetRootRef(ctx)
	return next, err
}
