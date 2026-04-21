package transport

import (
	"strconv"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/peer"
)

// LookupTransport is a directive to lookup running transports.
// Value type: transport.Transport.
type LookupTransport interface {
	// Directive indicates LookupTransport is a directive.
	directive.Directive

	// LookupTransportPeerIDConstraint returns a specific node ID we are looking for.
	// Can be empty.
	LookupTransportPeerIDConstraint() peer.ID
	// LookupTransportIDConstraint returns a specific transport ID we are looking for.
	// Can be empty.
	LookupTransportIDConstraint() uint64
}

// lookupTransport implements LookupTransport
type lookupTransport struct {
	peerIDConstraint      peer.ID
	transportIDConstraint uint64
}

// NewLookupTransport constructs a new LookupTransport directive.
func NewLookupTransport(peerID peer.ID, transportID uint64) LookupTransport {
	return &lookupTransport{
		peerIDConstraint:      peerID,
		transportIDConstraint: transportID,
	}
}

// LookupTransportPeerIDConstraint returns a specific peer ID node we are looking for.
// If empty, any node is matched.
func (d *lookupTransport) LookupTransportPeerIDConstraint() peer.ID {
	return d.peerIDConstraint
}

// LookupTransportIDConstraint returns a specific transport ID we are looking for.
// If empty, any transport id is matched.
func (d *lookupTransport) LookupTransportIDConstraint() uint64 {
	return d.transportIDConstraint
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupTransport) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupTransport) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupTransport) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupTransport)
	if !ok {
		return false
	}

	return d.LookupTransportIDConstraint() == od.LookupTransportIDConstraint() &&
		d.LookupTransportPeerIDConstraint() == od.LookupTransportPeerIDConstraint()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupTransport) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupTransport) GetName() string {
	return "LookupTransport"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupTransport) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if tpt := d.LookupTransportIDConstraint(); tpt != 0 {
		vals["transport-id"] = []string{strconv.FormatUint(tpt, 10)}
	}
	if nod := d.LookupTransportPeerIDConstraint(); nod != peer.ID("") {
		peerID := d.LookupTransportPeerIDConstraint().String()
		vals["peer-id"] = []string{peerID}
	}
	return vals
}

// _ is a type assertion
var _ LookupTransport = ((*lookupTransport)(nil))
