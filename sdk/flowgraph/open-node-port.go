package s4wave_flowgraph

import (
	"context"
	"strings"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// OpenNodePort returns an RPC client for a node's stream port in the granted graph.
// Each call reads the placement again and lasts until its calling context ends.
func OpenNodePort(graph SRPCFlowgraphResourceServiceClient, nodeID, port string) (srpc.Client, error) {
	if !validID(nodeID) || !validID(port) {
		return nil, errors.New("node and port must be nonempty path segments")
	}
	return rpcstream.NewRpcStreamClient(graph.OpenNodePort, nodeID+"/"+port, true), nil
}

// OpenNodePort resolves one node's port and carries its SRPC bytes to the plugin.
func (r *FlowgraphResource) OpenNodePort(strm SRPCFlowgraphResourceService_OpenNodePortStream) error {
	return rpcstream.HandleRpcStream(strm, func(ctx context.Context, route string, released func()) (srpc.Invoker, func(), error) {
		// Parse the endpoint within this mounted graph.
		nodeID, port, ok := strings.Cut(route, "/")
		if !ok {
			return nil, nil, errors.New("port route must be <node>/<port>")
		}
		protocolID, err := NodePortProtocolID(r.key, nodeID, port)
		if err != nil {
			return nil, nil, err
		}

		// Read the granted placement before acquiring a network stream.
		target, err := r.nodePortPeer(ctx, nodeID, port)
		if err != nil {
			return nil, nil, err
		}

		// Bind stream ownership to the granting Session, never a plugin-selected peer.
		if r.bus == nil || r.sessionPeerID == "" {
			return nil, nil, errors.New("Flowgraph port requires a granting Session")
		}
		session, releaseSession, err := transport.ResolveSessionTransport(ctx, r.bus, r.sessionPeerID, released)
		if err != nil {
			return nil, nil, err
		}
		if session == nil {
			releaseSession()
			return nil, nil, errors.New("Flowgraph port requires a running Session transport")
		}

		// Borrow the live bus before exposing the port, releasing a stopped Session.
		child := session.GetChildBus()
		if child == nil {
			releaseSession()
			return nil, nil, errors.New("Session transport stopped")
		}

		// Let the existing SRPC stream client own packet framing and link references.
		client := srpc.NewClient(stream_srpc.NewOpenStreamFunc(child, protocolID, session.GetPeerID(), target, 0))
		return srpc.NewClientInvoker(client), releaseSession, nil
	})
}

// nodePortPeer reads the target Device identity in one granted snapshot.
func (r *FlowgraphResource) nodePortPeer(ctx context.Context, nodeID, port string) (peer.ID, error) {
	// Read this graph and only the target node's derived binding.
	ws, release, err := r.readState(ctx)
	if err != nil {
		return "", err
	}
	defer release()

	// Decode the addressed graph and require the named node.
	snapshot, err := ReadFlowgraph(ctx, ws, r.key)
	if err != nil {
		return "", err
	}
	node := snapshot.GetState().GetNodes()[nodeID]
	if node == nil {
		return "", errors.New("Flowgraph node not found")
	}

	// Derive only this node's ports, including an object binding when present.
	bindings, err := ReadBindings(ctx, ws, r.bus, r.engineID, &Flowgraph{Nodes: map[string]*FlowgraphNode{nodeID: node}})
	if err != nil {
		return "", err
	}
	if findNodePort(snapshot.GetState(), bindings, nodeID, port).GetTypeId() != "stream" {
		return "", errors.New("Flowgraph stream port not found")
	}

	// Require a Device placement with its typed Device body in the same snapshot.
	placement := snapshot.GetPlacements()[nodeID]
	if placement.GetKind() != FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE {
		return "", errors.New("Flowgraph node is not placed on a Device")
	}
	if err := world_types.CheckObjectType(ctx, ws, placement.GetObjectKey(), s4wave_device.DeviceTypeID); err != nil {
		return "", err
	}

	// Retain the Device only while reading its authenticated peer identity.
	obj, found, err := ws.GetObject(ctx, placement.GetObjectKey())
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return "", err
	}
	if !found {
		return "", world.ErrObjectNotFound
	}

	// Decode the Device peer from the same World snapshot.
	device, err := s4wave_world.ReadWorldBlock[s4wave_device.Device](ctx, obj)
	if err != nil {
		return "", err
	}
	return peer.IDB58Decode(device.GetPeerId())
}
