// Package s4wave_flowgraph stores and edits authored compositions in a World.
package s4wave_flowgraph

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

const (
	// TCPPortNodeTypeID selects a Device's TCP stream output.
	TCPPortNodeTypeID = "tcp-port"
	// LocalPortNodeTypeID selects a Device's local stream listener.
	LocalPortNodeTypeID = "local-port"
	// CheckoutRootNodeTypeID selects a checkout root a Device advertises.
	CheckoutRootNodeTypeID = "checkout-root"
	// RemoteShellNodeTypeID selects the remote shell a Device serves.
	RemoteShellNodeTypeID = "remote-shell"
	// ForgeWorkerNodeTypeID selects the Forge Worker a Device hosts.
	ForgeWorkerNodeTypeID = "forge-worker"
	// StepNodeTypeID selects a node that runs once per activation.
	StepNodeTypeID = "step"
	// EventSourceNodeTypeID selects a node bound to an object that serves
	// EventSourceService. The standing run's controller turns the object's
	// events into arrivals.
	EventSourceNodeTypeID = "flowgraph/event-source"
)

// FlowgraphObjectKey builds an independently addressed Flowgraph key.
func FlowgraphObjectKey(id string) (string, error) {
	if !validID(id) {
		return "", errors.New("flowgraph id must be a nonempty path segment")
	}
	return FlowgraphTypeID + "/" + id, nil
}

// ParseFlowgraphObjectKey returns the ID in a Flowgraph object key.
func ParseFlowgraphObjectKey(key string) (string, error) {
	id, found := strings.CutPrefix(key, FlowgraphTypeID+"/")
	if !found || !validID(id) {
		return "", errors.New("flowgraph object key must have the form flowgraph/<id>")
	}
	return id, nil
}

// validID reports whether an identity is one nonempty, trimmed path segment.
func validID(id string) bool {
	return id != "" && id != "." && id != ".." &&
		strings.TrimSpace(id) == id && !strings.ContainsAny(id, "/\x00")
}

// NewFlowgraphBlock constructs the stored authored body.
func NewFlowgraphBlock() block.Block {
	return &Flowgraph{}
}

// UnmarshalFlowgraph reads a Flowgraph from a block cursor.
func UnmarshalFlowgraph(ctx context.Context, cursor *block.Cursor) (*Flowgraph, error) {
	return block.UnmarshalBlock[Flowgraph](ctx, cursor)
}

// Validate checks node identities, port contracts, and connection endpoints
// against the ports each node authors.
func (g *Flowgraph) Validate() error {
	return g.ValidateBound(nil, nil)
}

// ValidateBound checks the graph as Validate does, except that a node bound to
// an object also offers the ports the object's type declares while its binding
// is BOUND, so a connection may name either an authored or a declared port.
// A declared port wins over an authored port of the same name. A node with no
// BOUND binding offers its authored ports only, but a connection that is
// unchanged from previous keeps standing when it names a port that cannot be
// read now, since the object or its plugin may return. bindings is the
// snapshot's bindings by node ID; previous is the graph before the edit, or nil.
func (g *Flowgraph) ValidateBound(bindings map[string]*FlowgraphBinding, previous *Flowgraph) error {
	// Validate every node before resolving connection endpoints.
	for id, node := range g.GetNodes() {
		if !validID(id) {
			return errors.Errorf("invalid flowgraph node id %q", id)
		}
		if err := node.Validate(); err != nil {
			return errors.Wrapf(err, "node %q", id)
		}
	}

	// Require directed, type-compatible endpoints for every connection.
	for id, connection := range g.GetConnections() {
		if !validID(id) {
			return errors.Errorf("invalid flowgraph connection id %q", id)
		}
		if keepsStanding(g, previous, bindings, id, connection) {
			continue
		}
		out := findNodePort(g, bindings, connection.GetOutputNode(), connection.GetOutputPort())
		in := findNodePort(g, bindings, connection.GetInputNode(), connection.GetInputPort())
		if out.GetDirection() != FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT {
			return errors.Errorf("connection %q requires an output port", id)
		}
		if in.GetDirection() != FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT {
			return errors.Errorf("connection %q requires an input port", id)
		}
		if out.GetTypeId() != in.GetTypeId() {
			return errors.Errorf("connection %q has incompatible port types", id)
		}
	}
	return nil
}

// findNodePort returns a port a connection may name on a node: the declared
// port of a BOUND binding first, then the authored port.
func findNodePort(g *Flowgraph, bindings map[string]*FlowgraphBinding, nodeID, name string) *FlowgraphPort {
	if binding := bindings[nodeID]; binding.GetState() == FlowgraphBindingState_FLOWGRAPH_BINDING_STATE_BOUND {
		if port := binding.FindPort(name); port != nil {
			return port
		}
	}
	return g.GetNodes()[nodeID].FindPort(name)
}

// keepsStanding reports whether a connection is unchanged from previous and
// ends at a node bound to an object whose declared ports are unreadable, so the
// connection stays without a check.
func keepsStanding(g, previous *Flowgraph, bindings map[string]*FlowgraphBinding, id string, connection *FlowgraphConnection) bool {
	if previous == nil || !connection.EqualVT(previous.GetConnections()[id]) {
		return false
	}
	for _, nodeID := range []string{connection.GetOutputNode(), connection.GetInputNode()} {
		if g.GetNodes()[nodeID].GetObjectKey() == "" {
			continue
		}
		if bindings[nodeID].GetState() != FlowgraphBindingState_FLOWGRAPH_BINDING_STATE_BOUND {
			return true
		}
	}
	return false
}

// _ is a type assertion.
var _ block.Block = (*Flowgraph)(nil)
