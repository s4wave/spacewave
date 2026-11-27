package s4wave_flowgraph

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupFlowgraphNodeType is a directive to look up a Flowgraph node type for
// the nodes of one World. A Space plugin registers its node types for its own
// installation, so the World engine ID selects that installation's types.
// Core node types serve every World.
type LookupFlowgraphNodeType interface {
	// Directive indicates LookupFlowgraphNodeType is a directive.
	directive.Directive

	// LookupFlowgraphNodeTypeEngineID returns the ID of the World engine that
	// holds the Flowgraph.
	LookupFlowgraphNodeTypeEngineID() string
	// LookupFlowgraphNodeTypeID returns the node type ID to look up.
	LookupFlowgraphNodeTypeID() string
}

// LookupFlowgraphNodeTypeValue is the result type for LookupFlowgraphNodeType.
type LookupFlowgraphNodeTypeValue = FlowgraphNodeType

// ExLookupFlowgraphNodeType looks up a node type for the World engineID on the
// bus. It waits until a controller supplies the type, so a type from a
// controller that loads later resolves then. Release the reference when done
// with the node type.
func ExLookupFlowgraphNodeType(
	ctx context.Context,
	b bus.Bus,
	engineID, typeID string,
) (FlowgraphNodeType, directive.Reference, error) {
	av, _, avRef, err := bus.ExecOneOffTyped[LookupFlowgraphNodeTypeValue](ctx, b, NewLookupFlowgraphNodeType(engineID, typeID), nil, nil)
	if err != nil {
		return nil, nil, err
	}
	return av.GetValue(), avRef, nil
}

// lookupFlowgraphNodeType implements LookupFlowgraphNodeType.
type lookupFlowgraphNodeType struct {
	// engineID is the ID of the World engine that holds the Flowgraph.
	engineID string
	// typeID is the node type ID to look up.
	typeID string
}

// NewLookupFlowgraphNodeType constructs a new LookupFlowgraphNodeType directive.
func NewLookupFlowgraphNodeType(engineID, typeID string) LookupFlowgraphNodeType {
	return &lookupFlowgraphNodeType{engineID: engineID, typeID: typeID}
}

// Validate validates the directive.
func (d *lookupFlowgraphNodeType) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupFlowgraphNodeType) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// LookupFlowgraphNodeTypeEngineID returns the ID of the World engine that
// holds the Flowgraph.
func (d *lookupFlowgraphNodeType) LookupFlowgraphNodeTypeEngineID() string {
	return d.engineID
}

// LookupFlowgraphNodeTypeID returns the node type ID to look up.
func (d *lookupFlowgraphNodeType) LookupFlowgraphNodeTypeID() string {
	return d.typeID
}

// IsEquivalent checks if the other directive looks up the same node type.
func (d *lookupFlowgraphNodeType) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupFlowgraphNodeType)
	return ok && d.engineID == od.LookupFlowgraphNodeTypeEngineID() && d.typeID == od.LookupFlowgraphNodeTypeID()
}

// Superceeds checks if the directive overrides another.
func (d *lookupFlowgraphNodeType) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
func (d *lookupFlowgraphNodeType) GetName() string {
	return "LookupFlowgraphNodeType"
}

// GetDebugVals returns the directive arguments stringified.
func (d *lookupFlowgraphNodeType) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.engineID != "" {
		vals["engine-id"] = []string{d.engineID}
	}
	if d.typeID != "" {
		vals["flowgraph-node-type-id"] = []string{d.typeID}
	}
	return vals
}

// _ is a type assertion
var _ LookupFlowgraphNodeType = (*lookupFlowgraphNodeType)(nil)
