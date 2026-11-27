package flowgraph_registry

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
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
// streams each change of its run status, and of the node status the running
// controller shows, until the host ends the call or the controller fails.
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

	// Follow the controller the loader runs for the config. The directive is
	// disposed when the loader gives up on it.
	var bcast broadcast.Broadcast
	var running controller.Controller
	var runErr error
	_, ref, err := r.b.AddDirective(ctx, resolver.NewLoadControllerWithConfig(resolved.GetConfig()), bus.NewCallbackHandler(
		func(val directive.AttachedValue) {
			value, ok := val.GetValue().(resolver.LoadControllerWithConfigValue)
			if !ok || value == nil {
				return
			}
			bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				running, runErr = value.GetController(), value.GetError()
				broadcast()
			})
		},
		func(val directive.AttachedValue) {
			value, ok := val.GetValue().(resolver.LoadControllerWithConfigValue)
			if !ok || value == nil {
				return
			}
			bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if running == value.GetController() {
					running = nil
					broadcast()
				}
			})
		},
		cancel,
	))
	if err != nil {
		return err
	}
	defer ref.Release()

	// Send each change; a host that stops reading stops the controller.
	var sent *ExecControllerResponse
	for {
		var ctrl controller.Controller
		var ctrlErr error
		var changed <-chan struct{}
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ctrl, ctrlErr, changed = running, runErr, getWaitCh()
		})
		resp, nodeChanged := execStatus(ctrl, ctrlErr)
		if !resp.EqualVT(sent) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			sent = resp
		}
		if ctrlErr != nil {
			return ctrlErr
		}
		select {
		case <-ctx.Done():
			if strm.Context().Err() == nil {
				return errors.New("plugin dropped the controller")
			}
			return nil
		case <-changed:
		case <-nodeChanged:
		}
	}
}

// execStatus returns the status a controller's state reports, and a channel
// closed at the next change of the node status it shows, nil when it shows none.
func execStatus(ctrl controller.Controller, err error) (*ExecControllerResponse, <-chan struct{}) {
	// A failed or not yet running controller shows no node status.
	switch {
	case err != nil:
		return &ExecControllerResponse{Status: controller_exec.ControllerStatus_ControllerStatus_ERROR}, nil
	case ctrl == nil:
		return &ExecControllerResponse{Status: controller_exec.ControllerStatus_ControllerStatus_CONFIGURING}, nil
	}

	// A running controller shows the node status it implements, if any.
	resp := &ExecControllerResponse{Status: controller_exec.ControllerStatus_ControllerStatus_RUNNING}
	status, ok := ctrl.(s4wave_flowgraph.FlowgraphNodeStatus)
	if !ok {
		return resp, nil
	}
	state, detail, changed := status.GetNodeStatus()
	resp.NodeStatus = &NodeStatus{State: state, Detail: detail}
	return resp, changed
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
