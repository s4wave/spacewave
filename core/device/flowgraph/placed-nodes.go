package device_flowgraph

import (
	"context"
	"maps"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// readPlacedNodes reads every node of every Flowgraph in ws that is placed on
// the Device at deviceKey, in Flowgraph key and node ID order.
func readPlacedNodes(
	ctx context.Context,
	ws world.WorldState,
	deviceKey, devicePeerID string,
) ([]*s4wave_flowgraph.PlacedFlowgraphNode, error) {
	// List the Flowgraphs in key order.
	flowgraphKeys, err := world_types.ListObjectsWithType(ctx, ws, s4wave_flowgraph.FlowgraphTypeID)
	if err != nil {
		return nil, err
	}
	slices.Sort(flowgraphKeys)

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
				Connections:  connections,
			})
		}
	}
	return placed, nil
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
