package s4wave_flowgraph

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

const (
	// DevicePlacementPredicate links a Flowgraph to a Device, labelled by node ID.
	DevicePlacementPredicate = "<flowgraph/device>"
	// ActorPlacementPredicate links a Flowgraph to an actor, labelled by node ID.
	ActorPlacementPredicate = "<flowgraph/actor>"
)

// PlacementQuad constructs the graph edge for one node's placement.
func PlacementQuad(graphKey, nodeID string, placement *FlowgraphPlacement) (world.GraphQuad, error) {
	var predicate string
	switch placement.GetKind() {
	case FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE:
		predicate = DevicePlacementPredicate
	case FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR:
		predicate = ActorPlacementPredicate
	default:
		return nil, errors.New("placement kind is required")
	}
	return world.NewGraphQuadWithKeys(graphKey, predicate, placement.GetObjectKey(), strconv.Quote(nodeID)), nil
}

// ReadPlacements reads the placement edges of this graph, keyed by node ID.
func ReadPlacements(ctx context.Context, ws world.WorldState, graphKey string) (map[string]*FlowgraphPlacement, error) {
	// Query the two placement predicates without scanning other World objects.
	// A Flowgraph has one edge per node, so the lookups take no limit; the
	// batch lookup requires one over a remote World.
	var edges []world.GraphQuad
	for _, predicate := range []string{DevicePlacementPredicate, ActorPlacementPredicate} {
		found, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(graphKey, predicate, "", ""), 0)
		if err != nil {
			return nil, err
		}
		edges = append(edges, found...)
	}

	// Decode each edge's destination and graph-local node identity.
	placements := make(map[string]*FlowgraphPlacement)
	for _, edge := range edges {
		nodeID, placement, err := parsePlacementQuad(edge)
		if err != nil {
			return nil, err
		}
		if _, exists := placements[nodeID]; exists {
			return nil, errors.Errorf("node %q has multiple placements", nodeID)
		}
		placements[nodeID] = placement
	}
	return placements, nil
}

// parsePlacementQuad decodes a placement edge into its node ID and placement.
// It returns a nil placement for an edge with another predicate.
func parsePlacementQuad(edge world.GraphQuad) (string, *FlowgraphPlacement, error) {
	// Select the placement kind from the predicate.
	var kind FlowgraphPlacementKind
	switch edge.GetPredicate() {
	case DevicePlacementPredicate:
		kind = FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE
	case ActorPlacementPredicate:
		kind = FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR
	default:
		return "", nil, nil
	}

	// Decode the node ID label and the destination key.
	nodeID, err := strconv.Unquote(edge.GetLabel())
	if err != nil {
		return "", nil, err
	}
	key, err := world.GraphValueToKey(edge.GetObj())
	if err != nil {
		return "", nil, err
	}
	return nodeID, &FlowgraphPlacement{Kind: kind, ObjectKey: key}, nil
}

// ValidatePlacement checks the destination without interpreting actor objects.
func ValidatePlacement(ctx context.Context, ws world.WorldState, node *FlowgraphNode, placement *FlowgraphPlacement) error {
	// Require a destination object before checking the placement's execution kind.
	exists, err := ws.HasObject(ctx, placement.GetObjectKey())
	if err != nil {
		return err
	}
	if !exists {
		return world.ErrObjectNotFound
	}

	// Require Device objects for Device placements and Steps for actor placements.
	switch placement.GetKind() {
	case FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE:
		return world_types.CheckObjectType(ctx, ws, placement.GetObjectKey(), s4wave_device.DeviceTypeID)
	case FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR:
		if node.GetStep() == nil {
			return errors.New("actor placement requires a Step")
		}
		return nil
	default:
		return errors.New("placement kind is required")
	}
}
