package s4wave_flowgraph

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// HoldPredicate links a Flowgraph to the Space object one of its nodes holds,
// labelled by node ID.
//
// A node runs on a Device when it is a standing node, not a Step, with a Device
// placement or requirements a Device may claim. Such a node bound to an object
// holds that object: no other node in any Flowgraph of the World may hold it,
// and the node runs one copy. A node type whose work must have one admission
// point per Space, such as a service that takes custody of the Space's inputs,
// binds its node to an object it creates once per Space, so the World refuses
// a second placement of it on the graph mutation path, on every Device alike.
// An unplaced node, such as an event source, binds without holding, and so
// does a node whose binding is a read, such as a Device following a release
// that many Devices follow.
//
// Deleting the held object or its Flowgraph deletes the edge and releases the
// hold.
const HoldPredicate = "<flowgraph/holds>"

// HoldQuad constructs the edge recording that the node nodeID of the Flowgraph
// at graphKey holds the object at objectKey.
func HoldQuad(graphKey, nodeID, objectKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(graphKey, HoldPredicate, objectKey, strconv.Quote(nodeID))
}

// LookupHolder returns the Flowgraph key and node ID holding the object at
// objectKey, or empty strings when no node holds it.
func LookupHolder(ctx context.Context, ws world.WorldState, objectKey string) (string, string, error) {
	// Read the one edge that ends at the object.
	edges, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("", HoldPredicate, objectKey, ""), 1)
	if err != nil || len(edges) == 0 {
		return "", "", err
	}

	// Decode the holding Flowgraph and node.
	graphKey, err := world.GraphValueToKey(edges[0].GetSubject())
	if err != nil {
		return "", "", err
	}
	nodeID, err := strconv.Unquote(edges[0].GetLabel())
	if err != nil {
		return "", "", err
	}
	return graphKey, nodeID, nil
}

// HoldsObject reports whether node, placed at placements, holds its bound
// object.
func HoldsObject(node *FlowgraphNode, placements *FlowgraphPlacements) bool {
	if node.GetObjectKey() == "" || node.GetStep() != nil || node.GetReads() {
		return false
	}
	if node.GetRequirements() != nil {
		return true
	}
	return slices.ContainsFunc(placements.GetCopies(), func(p *FlowgraphPlacement) bool {
		return p.GetKind() == FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE
	})
}

// graphHolds returns the objects the snapshot's nodes hold, mapped to the
// holding node ID. The error reports two nodes holding one object, or a
// holding node that would run more than one copy. The map is complete either
// way.
func graphHolds(snapshot *FlowgraphSnapshot) (map[string]string, error) {
	holds := make(map[string]string)
	var err error
	for _, id := range slices.Sorted(maps.Keys(snapshot.GetState().GetNodes())) {
		// Skip a node that does not run on a Device or binds no object.
		node := snapshot.GetState().GetNodes()[id]
		placements := snapshot.GetPlacements()[id]
		if !HoldsObject(node, placements) {
			continue
		}

		// A held object has one admission point: one node and one copy.
		if len(placements.GetCopies()) > 1 || node.GetRequirements().GetReplicas() > 1 {
			err = cmp.Or(err, errors.Errorf("node %q holds object %q and runs one copy", id, node.GetObjectKey()))
		}
		if other, ok := holds[node.GetObjectKey()]; ok {
			err = cmp.Or(err, errors.Errorf("nodes %q and %q both hold object %q", other, id, node.GetObjectKey()))
			continue
		}
		holds[node.GetObjectKey()] = id
	}
	return holds, err
}

// Validate requires an independently addressed Flowgraph key.
func (o *SyncFlowgraphHoldsOp) Validate() error {
	_, err := ParseFlowgraphObjectKey(o.GetObjectKey())
	return err
}

// ApplyWorldOp brings the hold edges of the Flowgraph in line with its body and
// placements in ws. The edges are derived from ws alone, never from the
// author's view, so a replay that runs the op after another Device's commit
// refuses the hold that Device took first.
func (o *SyncFlowgraphHoldsOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, _ peer.ID) (bool, error) {
	// Derive the holds the graph's nodes take now.
	if err := o.Validate(); err != nil {
		return false, err
	}
	key := o.GetObjectKey()
	snapshot, err := ReadFlowgraph(ctx, ws, key)
	if err != nil {
		return false, err
	}
	holds, err := graphHolds(snapshot)
	if err != nil {
		return false, err
	}

	// Release the edges no node takes any more first, so a hold moved within
	// the graph does not find its old edge.
	edges, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, HoldPredicate, "", ""), 0)
	if err != nil {
		return false, err
	}
	for _, edge := range edges {
		objectKey, err := world.GraphValueToKey(edge.GetObj())
		if err != nil {
			return false, err
		}
		nodeID, err := strconv.Unquote(edge.GetLabel())
		if err == nil && holds[objectKey] == nodeID {
			continue
		}
		if err := ws.DeleteGraphQuad(ctx, edge); err != nil {
			return false, err
		}
	}

	// Keep or take each hold only while no other node holds the object. A
	// deleted and re-created object lost its edge, so a kept hold is checked
	// too.
	for objectKey, nodeID := range holds {
		graphKey, holder, err := LookupHolder(ctx, ws, objectKey)
		if err != nil {
			return false, err
		}
		switch {
		case graphKey == key && holder == nodeID:
			continue
		case graphKey != "":
			return false, errors.Errorf("node %q: object %q is held by node %q of Flowgraph %q", nodeID, objectKey, holder, graphKey)
		}
		if err := ws.SetGraphQuad(ctx, HoldQuad(key, nodeID, objectKey)); err != nil {
			return false, err
		}
	}
	return false, nil
}

// syncHolds applies a SyncFlowgraphHoldsOp to ws when previous or next holds
// an object, so an edit of a graph that holds nothing records no op.
func syncHolds(ctx context.Context, ws world.WorldState, key string, previous, next *FlowgraphSnapshot) error {
	// A stored violation must not keep the edit that removes it from syncing.
	before, _ := graphHolds(previous)
	after, _ := graphHolds(next)
	if len(before) == 0 && len(after) == 0 {
		return nil
	}
	_, _, err := ws.ApplyWorldOp(ctx, &SyncFlowgraphHoldsOp{ObjectKey: key}, "")
	return err
}

// CheckHold returns an error unless the node nodeID of the Flowgraph at
// graphKey, placed on a Device, holds its bound object in ws, binds none, or
// reads the object. A Device runs a holding node only while the World
// identifies the node as the holder, so a node whose object was deleted and
// then taken by another node does not run beside that node.
func CheckHold(ctx context.Context, ws world.WorldState, graphKey, nodeID string, node *FlowgraphNode) error {
	// A placed standing node holds the object it binds, unless it reads it.
	if node.GetObjectKey() == "" || node.GetStep() != nil || node.GetReads() {
		return nil
	}

	// Require the World's edge to reference this node.
	holderGraph, holder, err := LookupHolder(ctx, ws, node.GetObjectKey())
	if err != nil {
		return err
	}
	if holderGraph == graphKey && holder == nodeID {
		return nil
	}
	if holderGraph == "" {
		return errors.Errorf("node does not hold object %q; edit its Flowgraph to take the hold again", node.GetObjectKey())
	}
	return errors.Errorf("object %q is held by node %q of Flowgraph %q", node.GetObjectKey(), holder, holderGraph)
}

// _ is a type assertion.
var _ world.Operation = (*SyncFlowgraphHoldsOp)(nil)
