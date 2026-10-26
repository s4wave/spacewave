package device_flowgraph

import (
	"context"
	"maps"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// readPlacedNodes reads every Flowgraph node in ws that is placed on the
// Device at deviceKey, in Flowgraph key and node ID order, each marked with
// the World engine engineID. It reads only the Flowgraphs with a placement
// edge to the Device.
func readPlacedNodes(
	ctx context.Context,
	ws world.WorldState,
	engineID, deviceKey, devicePeerID string,
) ([]*s4wave_flowgraph.PlacedFlowgraphNode, error) {
	// List the Flowgraphs placed on the Device in key order.
	flowgraphKeys, err := readPlacedFlowgraphKeys(ctx, ws, deviceKey)
	if err != nil {
		return nil, err
	}

	// Resolve each Device's peer ID once, since several connections share it.
	peerIDs := map[string]string{deviceKey: devicePeerID}
	peerID := func(key string) (string, error) {
		if id, ok := peerIDs[key]; ok {
			return id, nil
		}
		id, err := readDevicePeerID(ctx, ws, key)
		peerIDs[key] = id
		return id, err
	}

	// Collect the nodes of each Flowgraph that are placed on the Device.
	var placed []*s4wave_flowgraph.PlacedFlowgraphNode
	for _, flowgraphKey := range flowgraphKeys {
		snapshot, err := s4wave_flowgraph.ReadFlowgraph(ctx, ws, flowgraphKey)
		if err != nil {
			return nil, errors.Wrapf(err, "read flowgraph %q", flowgraphKey)
		}
		for _, nodeID := range slices.Sorted(maps.Keys(snapshot.GetPlacements())) {
			if !isPlacedOn(snapshot.GetPlacements()[nodeID], deviceKey) {
				continue
			}
			connections, err := placedConnections(snapshot, nodeID, peerID)
			if err != nil {
				return nil, err
			}
			placed = append(placed, &s4wave_flowgraph.PlacedFlowgraphNode{
				FlowgraphKey: flowgraphKey,
				NodeID:       nodeID,
				Node:         snapshot.GetState().GetNodes()[nodeID],
				DevicePeerID: devicePeerID,
				EngineID:     engineID,
				Connections:  connections,
			})
		}
	}
	return placed, nil
}

// readPlacedFlowgraphKeys returns the sorted keys of the Flowgraphs with a
// node placed on the Device at deviceKey, from the placement edges that end at
// the Device. A Device holds one edge per placed node, so the lookup takes no
// limit.
func readPlacedFlowgraphKeys(ctx context.Context, ws world.WorldState, deviceKey string) ([]string, error) {
	// Look up the placement edges that end at the Device.
	filter := world.NewGraphQuadWithKeys("", s4wave_flowgraph.DevicePlacementPredicate, deviceKey, "")
	edges, err := ws.LookupGraphQuads(ctx, filter, 0)
	if err != nil {
		return nil, err
	}

	// Collect each Flowgraph once.
	keys := make([]string, 0, len(edges))
	for _, edge := range edges {
		key, err := world.GraphValueToKey(edge.GetSubject())
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return slices.Compact(keys), nil
}

// placedConnections returns the connections of snapshot that end at nodeID, in
// connection ID order, each with the peer ID of the Device at its other end.
func placedConnections(
	snapshot *s4wave_flowgraph.FlowgraphSnapshot,
	nodeID string,
	peerID func(deviceKey string) (string, error),
) ([]*s4wave_flowgraph.PlacedFlowgraphConnection, error) {
	connections := snapshot.GetState().GetConnections()
	var placed []*s4wave_flowgraph.PlacedFlowgraphConnection
	for _, connectionID := range slices.Sorted(maps.Keys(connections)) {
		connection := connections[connectionID]
		var remoteNodeID string
		switch nodeID {
		case connection.GetOutputNode():
			remoteNodeID = connection.GetInputNode()
		case connection.GetInputNode():
			remoteNodeID = connection.GetOutputNode()
		default:
			continue
		}

		// An end that is not on a Device has no peer ID.
		var remotePeerID string
		if remote := snapshot.GetPlacements()[remoteNodeID]; remote.GetKind() == s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE {
			var err error
			remotePeerID, err = peerID(remote.GetObjectKey())
			if err != nil {
				return nil, err
			}
		}
		placed = append(placed, &s4wave_flowgraph.PlacedFlowgraphConnection{
			ID:                 connectionID,
			Connection:         connection,
			RemoteDevicePeerID: remotePeerID,
		})
	}
	return placed, nil
}

// isPlacedOn reports whether placement puts a node on the Device at deviceKey.
func isPlacedOn(placement *s4wave_flowgraph.FlowgraphPlacement, deviceKey string) bool {
	return placement.GetKind() == s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE &&
		placement.GetObjectKey() == deviceKey
}

// readDevicePeerID reads the peer ID of the Device object at key. It returns an
// empty ID for a missing object, so the nodes that connect to it fail to
// compile instead of stopping every other node.
func readDevicePeerID(ctx context.Context, ws world.WorldState, key string) (string, error) {
	// Treat a missing object as no peer.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil || !found {
		return "", err
	}
	device, err := readDevice(ctx, obj)
	if err != nil {
		return "", err
	}
	return device.GetPeerId(), nil
}

// readDevice reads the Device block of obj.
func readDevice(ctx context.Context, obj world.ObjectState) (*s4wave_device.Device, error) {
	var device *s4wave_device.Device
	_, _, err := world.AccessObjectState(ctx, obj, false, func(cursor *block.Cursor) error {
		var err error
		device, err = s4wave_device.UnmarshalDevice(ctx, cursor)
		return err
	})
	return device, err
}
