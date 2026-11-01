package s4wave_flowgraph

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
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
	state, err := s4wave_world.ReadWorldBlock[Flowgraph](ctx, obj)
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

// ReadFlowgraphRun reads a run's body and the object revision it was read at.
func ReadFlowgraphRun(ctx context.Context, ws world.WorldState, key string) (*FlowgraphRun, uint64, error) {
	// Retain the object while decoding the body of one root reference.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, 0, err
	}
	if !found {
		return nil, 0, world.ErrObjectNotFound
	}

	// Decode the run at the same revision the reference reports.
	ref, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, 0, err
	}
	var run *FlowgraphRun
	_, err = world.AccessObject(ctx, obj.AccessWorldState, ref, func(cursor *block.Cursor) error {
		var err error
		run, err = UnmarshalFlowgraphRun(ctx, cursor)
		return err
	})
	if err == nil && run == nil {
		err = errors.New("flowgraph run object is empty")
	}
	return run, rev, err
}

// UpdateFlowgraph changes the body and placement graph in the caller's transaction.
// The caller commits on success or discards the transaction on error. It has no
// object type registry, so a bound node offers its authored ports only.
func UpdateFlowgraph(ctx context.Context, ws world.WorldState, key string, request *UpdateFlowgraphRequest) (*FlowgraphSnapshot, error) {
	return UpdateBoundFlowgraph(ctx, ws, nil, "", key, request)
}

// UpdateBoundFlowgraph changes the graph as UpdateFlowgraph does, and checks
// connections to a node bound to an object against the ports the object's type
// declares on b, scoped to engineID. The returned snapshot carries the bindings
// read from the same transaction.
func UpdateBoundFlowgraph(ctx context.Context, ws world.WorldState, b bus.Bus, engineID, key string, request *UpdateFlowgraphRequest) (*FlowgraphSnapshot, error) {
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
		next.Placements = make(map[string]*FlowgraphPlacements)
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
	bindings, err := ReadBindings(ctx, ws, b, engineID, state)
	if err != nil {
		return nil, err
	}
	if err := state.ValidateBound(bindings, previous.GetState()); err != nil {
		return nil, err
	}
	next.Bindings = bindings

	// Apply authored placement edits before reconciling changed requirements.
	for id, placement := range request.GetSetPlacements() {
		next.Placements[id] = placement.CloneVT()
		next.Placements[id].Normalize()
		if len(next.Placements[id].GetCopies()) == 0 {
			delete(next.Placements, id)
		}
	}
	for _, id := range request.GetRemovePlacementNodeIds() {
		delete(next.Placements, id)
	}

	// A changed resource request releases every copy's grant before re-admission.
	// Count-only edits keep grants and let claimNode release the newest copies.
	for id, node := range state.Nodes {
		old := previous.GetState().GetNodes()[id].GetRequirements().CloneVT()
		wanted := node.GetRequirements().CloneVT()
		if old == nil {
			continue
		}
		old.Replicas = 0
		if wanted != nil {
			wanted.Replicas = 0
		}
		if wanted.EqualVT(old) {
			continue
		}

		// An explicit pin can retain an edge while relinquishing its grant.
		for _, placement := range previous.GetPlacements()[id].GetCopies() {
			if err := ReleaseNodeCapacity(ctx, ws, key, id, placement.GetObjectKey()); err != nil {
				return nil, err
			}
		}
		if _, set := request.GetSetPlacements()[id]; !set {
			delete(next.Placements, id)
		}
	}

	// Validate the placements that remain after requirements and pin edits.
	for id, group := range next.Placements {
		node, found := state.Nodes[id]
		if !found {
			return nil, errors.Errorf("placement refers to missing node %q", id)
		}
		if len(group.GetCopies()) > 1 && node.GetRequirements() == nil {
			return nil, errors.Errorf("node %q needs requirements for multiple placements", id)
		}
		for _, placement := range group.GetCopies() {
			if err := ValidatePlacement(ctx, ws, node, placement); err != nil {
				return nil, errors.Wrapf(err, "node %q", id)
			}
		}
	}

	// Remove replaced or deleted placement edges.
	var placed bool
	for id, group := range previous.GetPlacements() {
		kept := indexPlacements(next.Placements[id])
		for _, placement := range group.GetCopies() {
			if _, ok := kept[placementKey{placement.GetKind(), placement.GetObjectKey()}]; ok {
				continue
			}

			// Release the departing copy's grant with its edge.
			edge, err := PlacementQuad(key, id, placement)
			if err != nil {
				return nil, err
			}
			if err := ReleaseNodeCapacity(ctx, ws, key, id, placement.GetObjectKey()); err != nil {
				return nil, err
			}
			if err := ws.DeleteGraphQuad(ctx, edge); err != nil {
				return nil, err
			}
			placed = true
		}
	}

	// Add new placement edges in the same transaction as the authored body.
	for id, group := range next.GetPlacements() {
		old := indexPlacements(previous.Placements[id])
		for _, placement := range group.GetCopies() {
			if _, ok := old[placementKey{placement.GetKind(), placement.GetObjectKey()}]; ok {
				continue
			}

			// Publish the new destination in the authored edit.
			edge, err := PlacementQuad(key, id, placement)
			if err != nil {
				return nil, err
			}
			if err := ws.SetGraphQuad(ctx, edge); err != nil {
				return nil, err
			}
			placed = true
		}
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
