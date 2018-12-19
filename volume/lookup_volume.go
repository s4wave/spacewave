package volume

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupVolume is a directive to lookup running volumes.
// Value type: volume.Volume.
type LookupVolume interface {
	// Directive indicates LookupVolume is a directive.
	directive.Directive

	// LookupVolumePeerIDConstraint returns a specific node ID we are looking for.
	// Can be empty.
	LookupVolumePeerIDConstraint() peer.ID
}

// lookupVolume implements LookupVolume
type lookupVolume struct {
	peerIDConstraint peer.ID
}

// NewLookupVolume constructs a new LookupVolume directive.
func NewLookupVolume(peerID peer.ID) LookupVolume {
	return &lookupVolume{
		peerIDConstraint: peerID,
	}
}

// LookupVolumePeerIDConstraint returns a specific peer ID node we are looking for.
// If empty, any node is matched.
func (d *lookupVolume) LookupVolumePeerIDConstraint() peer.ID {
	return d.peerIDConstraint
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupVolume) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupVolume) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupVolume) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupVolume)
	if !ok {
		return false
	}

	return d.LookupVolumePeerIDConstraint() == od.LookupVolumePeerIDConstraint()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupVolume) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupVolume) GetName() string {
	return "LookupVolume"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupVolume) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if nod := d.LookupVolumePeerIDConstraint(); nod != peer.ID("") {
		peerID := d.LookupVolumePeerIDConstraint().Pretty()
		vals["peer-id"] = []string{peerID}
	}
	return vals
}

// _ is a type assertion
var _ LookupVolume = ((*lookupVolume)(nil))
