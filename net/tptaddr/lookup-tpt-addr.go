package tptaddr

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// LookupTptAddr is a directive to look up transport addresses for a peer id.
//
// Value: string in transport address format.
type LookupTptAddr interface {
	// Directive indicates LookupTptAddr is a directive.
	directive.Directive

	// LookupTptAddrTargetPeerId returns the target peer ID.
	// Cannot be empty.
	LookupTptAddrTargetPeerId() peer.ID
}

// LookupTptAddrValue is the type emitted when resolving LookupTptAddr.
type LookupTptAddrValue = string

// ExLookupTptAddr executes collecting transport addresses for a peer.
//
// Note: waits to return until all resolvers become idle.
func ExLookupTptAddr(ctx context.Context, b bus.Bus, destPeerID peer.ID, waitOne bool) ([]LookupTptAddrValue, directive.Instance, directive.Reference, error) {
	out, di, valsRef, err := bus.ExecCollectValues[LookupTptAddrValue](
		ctx,
		b,
		NewLookupTptAddr(destPeerID),
		waitOne,
		nil,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(out) == 0 {
		valsRef.Release()
		return nil, nil, nil, nil
	}
	return out, di, valsRef, nil
}

// lookupTptAddr implements LookupTptAddr
type lookupTptAddr struct {
	dest peer.ID
}

// NewLookupTptAddr constructs a new LookupTptAddr directive.
func NewLookupTptAddr(destPeer peer.ID) LookupTptAddr {
	return &lookupTptAddr{dest: destPeer}
}

// LookupTptAddrTargetPeerId returns the target peer ID.
func (d *lookupTptAddr) LookupTptAddrTargetPeerId() peer.ID {
	return d.dest
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *lookupTptAddr) Validate() error {
	if len(d.dest) == 0 {
		return errors.Wrap(peer.ErrEmptyPeerID, "destination")
	}
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupTptAddr) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *lookupTptAddr) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LookupTptAddr)
	if !ok {
		return false
	}

	return d.LookupTptAddrTargetPeerId() == od.LookupTptAddrTargetPeerId()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *lookupTptAddr) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupTptAddr) GetName() string {
	return "LookupTptAddr"
}

// GetDebugVals returns the directive arguments as k/v pairs.
// This is not necessarily unique, and is primarily intended for display.
func (d *lookupTptAddr) GetDebugVals() directive.DebugValues {
	vals := directive.NewDebugValues()
	vals["peer-id"] = []string{d.LookupTptAddrTargetPeerId().String()}
	return vals
}

// _ is a type constraint
var _ LookupTptAddr = ((*lookupTptAddr)(nil))
