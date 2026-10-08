// Package s4wave_flowgraph stores and edits authored compositions in a World.
package s4wave_flowgraph

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

const (
	// FlowgraphTypeID is the World type of an authored Flowgraph.
	FlowgraphTypeID = "flowgraph"
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
	return block.UnmarshalBlock[*Flowgraph](ctx, cursor, NewFlowgraphBlock)
}

// MarshalBlock encodes the authored body.
func (g *Flowgraph) MarshalBlock() ([]byte, error) {
	return g.MarshalVT()
}

// UnmarshalBlock decodes the authored body.
func (g *Flowgraph) UnmarshalBlock(data []byte) error {
	return g.UnmarshalVT(data)
}

// Validate checks node identities, port contracts, and connection endpoints.
func (g *Flowgraph) Validate() error {
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
		out := g.GetNodes()[connection.GetOutputNode()].FindPort(connection.GetOutputPort())
		in := g.GetNodes()[connection.GetInputNode()].FindPort(connection.GetInputPort())
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

// _ is a type assertion.
var _ block.Block = (*Flowgraph)(nil)
