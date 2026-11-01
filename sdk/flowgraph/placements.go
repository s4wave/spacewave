package s4wave_flowgraph

import (
	"cmp"
	"context"
	"slices"
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
	// ActorTemplatePlacementPredicate links a Step to its actor template.
	ActorTemplatePlacementPredicate = "<flowgraph/actor-template>"
)

// PlacementQuad constructs the graph edge for one node's placement.
func PlacementQuad(graphKey, nodeID string, placement *FlowgraphPlacement) (world.GraphQuad, error) {
	var predicate string
	switch placement.GetKind() {
	case FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE:
		predicate = DevicePlacementPredicate
	case FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR:
		predicate = ActorPlacementPredicate
	case FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR_TEMPLATE:
		predicate = ActorTemplatePlacementPredicate
	default:
		return nil, errors.New("placement kind is required")
	}
	return world.NewGraphQuadWithKeys(graphKey, predicate, placement.GetObjectKey(), strconv.Quote(nodeID)), nil
}

// ReadPlacements reads the placement edges of this graph, keyed by node ID.
func ReadPlacements(ctx context.Context, ws world.WorldState, graphKey string) (map[string]*FlowgraphPlacements, error) {
	// Query the placement predicates without scanning other World objects.
	// A Flowgraph has one edge per copy, so the lookups take no limit; the
	// batch lookup requires one over a remote World.
	var edges []world.GraphQuad
	for _, predicate := range []string{DevicePlacementPredicate, ActorPlacementPredicate, ActorTemplatePlacementPredicate} {
		found, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(graphKey, predicate, "", ""), 0)
		if err != nil {
			return nil, err
		}
		edges = append(edges, found...)
	}

	// Decode each edge's destination and graph-local node identity.
	placements := make(map[string]*FlowgraphPlacements)
	for _, edge := range edges {
		nodeID, placement, err := parsePlacementQuad(edge)
		if err != nil {
			return nil, err
		}
		if placements[nodeID] == nil {
			placements[nodeID] = &FlowgraphPlacements{}
		}
		placements[nodeID].Copies = append(placements[nodeID].Copies, placement)
	}

	// Sort destinations so readers select the same copy on every Device.
	for _, group := range placements {
		group.Normalize()
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
	case ActorTemplatePlacementPredicate:
		kind = FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR_TEMPLATE
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
	// A requirements-bearing node waits unplaced until an eligible Device claims it.
	if placement == nil && node.GetRequirements() != nil {
		return nil
	}

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
	case FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR, FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR_TEMPLATE:
		if node.GetStep() == nil {
			return errors.New("actor placement requires a Step")
		}
		return nil
	default:
		return errors.New("placement kind is required")
	}
}

// First returns the first destination in the canonical placement order.
// Single-destination consumers route to this copy.
func (p *FlowgraphPlacements) First() *FlowgraphPlacement {
	if len(p.GetCopies()) == 0 {
		return nil
	}
	return p.Copies[0]
}

// placementKey identifies an edge destination within one node's placement set.
type placementKey struct {
	// kind distinguishes Device and actor predicates.
	kind FlowgraphPlacementKind
	// key identifies the destination object.
	key string
}

// indexPlacements indexes a node's destinations for linear set differences.
func indexPlacements(group *FlowgraphPlacements) map[placementKey]struct{} {
	index := make(map[placementKey]struct{}, len(group.GetCopies()))
	for _, placement := range group.GetCopies() {
		index[placementKey{placement.GetKind(), placement.GetObjectKey()}] = struct{}{}
	}
	return index
}

// Normalize sorts and deduplicates destinations to match the graph edge set.
func (p *FlowgraphPlacements) Normalize() {
	if p == nil {
		return
	}
	slices.SortFunc(p.Copies, func(a, b *FlowgraphPlacement) int {
		return cmp.Or(cmp.Compare(a.GetKind(), b.GetKind()), cmp.Compare(a.GetObjectKey(), b.GetObjectKey()))
	})
	p.Copies = slices.CompactFunc(p.Copies, (*FlowgraphPlacement).EqualVT)
}
