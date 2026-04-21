package peer

import (
	"github.com/aperturerobotics/controllerbus/directive"
)

// GetPeer is a directive to lookup a peer on a controller.
type GetPeer interface {
	// Directive indicates GetPeer is a directive.
	directive.Directive

	// GetPeerIDConstraint returns a specific peer ID node we are looking for.
	// If empty, any node is matched.
	GetPeerIDConstraint() ID
}

// GetPeerValue is the result of the GetPeer directive.
type GetPeerValue = Peer

// getPeer implements GetPeer with a optional peer ID constraint.
type getPeer struct {
	peerIDConstraint ID
}

// NewGetPeer constructs a new getPeer directive.
func NewGetPeer(peerID ID) GetPeer {
	return &getPeer{peerIDConstraint: peerID}
}

// GetPeerIDConstraint returns a specific peer ID node we are looking for.
// If empty, any node is matched.
func (d *getPeer) GetPeerIDConstraint() ID {
	return d.peerIDConstraint
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *getPeer) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *getPeer) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *getPeer) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(GetPeer)
	if !ok {
		return false
	}

	return d.peerIDConstraint == od.GetPeerIDConstraint()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *getPeer) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *getPeer) GetName() string {
	return "GetPeer"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *getPeer) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if pid := d.GetPeerIDConstraint(); pid != ID("") {
		vals["peer-id"] = []string{pid.String()}
	}
	return vals
}

// _ is a type constraint
var _ GetPeer = ((*getPeer)(nil))

// _ is a type constraint
var _ directive.Debuggable = ((*getPeer)(nil))
