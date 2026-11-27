package flowgraph_registry

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// NodeTypeResource serves one plugin node type's compiler and controllers.
type NodeTypeResource struct {
	// b is the plugin's bus, which holds the factories for the plugin config IDs.
	b bus.Bus
	// metadata is the declaration registered for this node type.
	metadata *NodeType
	// nodeType implements compilation inside the plugin.
	nodeType s4wave_flowgraph.FlowgraphNodeType
}

// NewNodeTypeResource constructs the Resource for the node type metadata
// declares, running its plugin config IDs on b.
func NewNodeTypeResource(b bus.Bus, metadata *NodeType, nodeType s4wave_flowgraph.FlowgraphNodeType) *NodeTypeResource {
	return &NodeTypeResource{b: b, metadata: metadata.CloneVT(), nodeType: nodeType}
}

// Compile encodes the plugin's configs without assigning Device keys or revisions.
func (r *NodeTypeResource) Compile(ctx context.Context, req *CompileRequest) (*CompileResponse, error) {
	// Keep the granted compiler restricted to its declared node type.
	if req.GetNode().GetTypeId() != r.metadata.GetTypeId() {
		return nil, errors.New("node type does not match compiler")
	}
	compiled, err := r.nodeType.Compile(ctx, req.PlacedNode())
	if err != nil {
		return nil, err
	}

	// Use ControllerBus's binary config encoding for the language boundary.
	entries := make(map[string]*configset_proto.ControllerConfig, len(compiled))
	for name, conf := range compiled {
		encoded, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(0, conf), false)
		if err != nil {
			return nil, err
		}
		entries[name] = encoded
	}
	return &CompileResponse{Entries: entries}, nil
}

// ExecController runs one declared plugin config on the plugin's bus and
// streams its status until the host ends the call or the controller fails.
func (r *NodeTypeResource) ExecController(req *ExecControllerRequest, strm SRPCNodeTypeResourceService_ExecControllerStream) error {
	// Run only the configs this registration declares for the plugin.
	encoded := req.GetConfig()
	if !slices.Contains(r.metadata.GetPluginConfigIds(), encoded.GetId()) {
		return errors.Errorf("config ID %q is not a plugin config of this node type", encoded.GetId())
	}
	ctx, cancel := context.WithCancel(strm.Context())
	defer cancel()
	resolved, err := encoded.Resolve(ctx, r.b)
	if err != nil {
		return err
	}

	// Report each status change. A host that stops reading stops
	// the controller.
	return controller_exec.ExecuteController(ctx, r.b, resolved.GetConfig(), func(status controller_exec.ControllerStatus) {
		if err := strm.Send(&controller_exec.ExecControllerResponse{Id: encoded.GetId(), Status: status}); err != nil {
			cancel()
		}
	})
}

// NewCompileRequest copies a placement into its RPC representation.
func NewCompileRequest(node *s4wave_flowgraph.PlacedFlowgraphNode) *CompileRequest {
	// Copy the placement and all remote connection identities.
	req := &CompileRequest{
		FlowgraphKey:   node.FlowgraphKey,
		NodeId:         node.NodeID,
		Node:           node.Node.CloneVT(),
		DevicePeerId:   node.DevicePeerID,
		EngineId:       node.EngineID,
		SpaceId:        node.SpaceID,
		LaunchApproval: node.LaunchApproval.CloneVT(),
		Connections:    make([]*PlacedConnection, len(node.Connections)),
	}
	for i, conn := range node.Connections {
		req.Connections[i] = &PlacedConnection{
			Id:                 conn.ID,
			Connection:         conn.Connection.CloneVT(),
			RemoteDevicePeerId: conn.RemoteDevicePeerID,
		}
	}
	return req
}

// PlacedNode copies the RPC context into the compiler's local interface.
func (r *CompileRequest) PlacedNode() *s4wave_flowgraph.PlacedFlowgraphNode {
	// Restore the node and all remote connection identities.
	node := &s4wave_flowgraph.PlacedFlowgraphNode{
		FlowgraphKey:   r.GetFlowgraphKey(),
		NodeID:         r.GetNodeId(),
		Node:           r.GetNode().CloneVT(),
		DevicePeerID:   r.GetDevicePeerId(),
		EngineID:       r.GetEngineId(),
		SpaceID:        r.GetSpaceId(),
		LaunchApproval: r.GetLaunchApproval().CloneVT(),
		Connections:    make([]*s4wave_flowgraph.PlacedFlowgraphConnection, len(r.GetConnections())),
	}
	for i, conn := range r.GetConnections() {
		node.Connections[i] = &s4wave_flowgraph.PlacedFlowgraphConnection{
			ID:                 conn.GetId(),
			Connection:         conn.GetConnection().CloneVT(),
			RemoteDevicePeerID: conn.GetRemoteDevicePeerId(),
		}
	}
	return node
}

// ServeRpc serves only declared services, routing each call to its port on the plugin bus.
func (r *NodeTypeResource) ServeRpc(strm SRPCNodeTypeResourceService_ServeRpcStream) error {
	return rpcstream.HandleRpcStream(strm, func(ctx context.Context, serverID string, released func()) (srpc.Invoker, func(), error) {
		return &portInvoker{invoker: bifrost_rpc.NewInvoker(r.b, serverID, true), services: r.metadata.GetPluginServiceIds()}, nil, nil
	})
}

// _ is a type assertion.
var _ SRPCNodeTypeResourceServiceServer = (*NodeTypeResource)(nil)
