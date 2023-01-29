package volume

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupVolume is a directive to lookup running volumes.
// Value type: volume.Volume.
type LookupVolume interface {
	// Directive indicates LookupVolume is a directive.
	directive.Directive

	// LookupVolumeID returns a specific volume ID to filter to.
	// Can be empty.
	LookupVolumeID() string
	// LookupVolumePeerIDConstraint returns a specific peer ID we are looking for.
	// Can be empty.
	LookupVolumePeerIDConstraint() peer.ID
}

// LookupVolumeValue is the value type for LookupVolume.
type LookupVolumeValue = Volume

// lookupVolume implements LookupVolume
type lookupVolume struct {
	volumeID         string
	peerIDConstraint peer.ID
}

// NewLookupVolume constructs a new LookupVolume directive.
// both parameters can be empty
func NewLookupVolume(volumeID string, peerID peer.ID) LookupVolume {
	return &lookupVolume{
		volumeID:         volumeID,
		peerIDConstraint: peerID,
	}
}

// ExLookupVolume executes the LookupVolume directive returning one Volume.
// both parameters can be empty
func ExLookupVolume(
	ctx context.Context,
	b bus.Bus,
	volumeID string,
	peerID peer.ID,
	returnIfIdle bool,
) (LookupVolumeValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[LookupVolumeValue](
		ctx,
		b,
		NewLookupVolume(volumeID, peerID),
		returnIfIdle,
		nil,
		nil,
	)
}

// CheckLookupMatchesVolume checks if a lookupvolume matches a volume.
// only checks if there are any constraints set on the directive (not ID).
func CheckLookupMatchesVolume(dir LookupVolume, vol Volume, aliases []string) bool {
	if peerIDConstraint := dir.LookupVolumePeerIDConstraint(); len(peerIDConstraint) != 0 {
		if vol.GetPeerID() != peerIDConstraint {
			return false
		}
	}
	if !CheckIDMatchesAliases(dir.LookupVolumeID(), vol.GetID(), aliases) {
		return false
	}

	return true
}

// LookupVolumeID returns a specific volume ID to filter to.
// Can be empty.
func (d *lookupVolume) LookupVolumeID() string {
	return d.volumeID
}

// LookupVolumePeerIDConstraint returns a specific peer ID node we are looking for.
// If empty, any volume is matched.
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

	return d.LookupVolumePeerIDConstraint() == od.LookupVolumePeerIDConstraint() &&
		d.LookupVolumeID() == od.LookupVolumeID()
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
	if vid := d.LookupVolumeID(); vid != "" {
		vals["volume-id"] = []string{vid}
	}
	return vals
}

// _ is a type assertion
var _ LookupVolume = ((*lookupVolume)(nil))
