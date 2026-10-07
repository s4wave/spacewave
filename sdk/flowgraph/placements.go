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
	filters := []world.GraphQuad{
		world.NewGraphQuadWithKeys(graphKey, DevicePlacementPredicate, "", ""),
		world.NewGraphQuadWithKeys(graphKey, ActorPlacementPredicate, "", ""),
	}
	sets, err := ws.LookupGraphQuadsBatch(ctx, filters, 0)
	if err != nil {
		return nil, err
	}

	// Decode each edge's destination and graph-local node identity.
	placements := make(map[string]*FlowgraphPlacement)
	for i, edges := range sets {
		kind := FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE
		if i == 1 {
			kind = FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR
		}
		for _, edge := range edges {
			nodeID, err := strconv.Unquote(edge.GetLabel())
			if err != nil {
				return nil, err
			}
			key, err := world.GraphValueToKey(edge.GetObj())
			if err != nil {
				return nil, err
			}
			if _, exists := placements[nodeID]; exists {
				return nil, errors.Errorf("node %q has multiple placements", nodeID)
			}
			placements[nodeID] = &FlowgraphPlacement{Kind: kind, ObjectKey: key}
		}
	}
	return placements, nil
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
