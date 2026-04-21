package tptaddr

import (
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
)

// DialTptAddr is a directive to establish a link with a peer via a transport address.
//
// Value: Link
type DialTptAddr interface {
	// Directive indicates DialTptAddr is a directive.
	directive.Directive

	// DialTptAddrSourcePeerId returns the source peer ID.
	// Can be empty to allow any.
	DialTptAddrSourcePeerId() peer.ID
	// DialTptAddrTargetPeerId returns the target peer ID.
	// Cannot be empty.
	DialTptAddrTargetPeerId() peer.ID
	// DialTptAddrDialerOpts are options for the dialer.
	// Address: usually {transport-type-id}|{addr}
	// Addr cannot be empty.
	DialTptAddrDialerOpts() *dialer.DialerOpts
}

// DialTptAddrValue is the type emitted when resolving DialTptAddr.
type DialTptAddrValue = link.Link

// dialTptAddr implements DialTptAddr
type dialTptAddr struct {
	dialerOpts *dialer.DialerOpts
	src, dest  peer.ID
}

// NewDialTptAddr constructs a new DialTptAddr directive.
func NewDialTptAddr(dialerOpts *dialer.DialerOpts, srcPeer, destPeer peer.ID) DialTptAddr {
	return &dialTptAddr{dialerOpts: dialerOpts, src: srcPeer, dest: destPeer}
}

// DialTptAddrDialerOpts are options for the dialer.
func (d *dialTptAddr) DialTptAddrDialerOpts() *dialer.DialerOpts {
	return d.dialerOpts
}

// DialTptAddrTargetPeerId returns the target peer ID.
func (d *dialTptAddr) DialTptAddrTargetPeerId() peer.ID {
	return d.dest
}

// DialTptAddrSourcePeerId returns the src peer ID.
func (d *dialTptAddr) DialTptAddrSourcePeerId() peer.ID {
	return d.src
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *dialTptAddr) Validate() error {
	if len(d.dest) == 0 {
		return errors.Wrap(peer.ErrEmptyPeerID, "destination")
	}
	if d.dialerOpts == nil {
		return errors.New("dialer options cannot be nil")
	}
	if d.dialerOpts.GetAddress() == "" {
		return errors.New("dialer address cannot be empty")
	}
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *dialTptAddr) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *dialTptAddr) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(DialTptAddr)
	if !ok {
		return false
	}

	return d.DialTptAddrTargetPeerId() == od.DialTptAddrTargetPeerId() &&
		d.DialTptAddrSourcePeerId() == od.DialTptAddrSourcePeerId() &&
		d.DialTptAddrDialerOpts().GetAddress() == od.DialTptAddrDialerOpts().GetAddress()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *dialTptAddr) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *dialTptAddr) GetName() string {
	return "DialTptAddr"
}

// GetDebugVals returns the directive arguments as k/v pairs.
// This is not necessarily unique, and is primarily intended for display.
func (d *dialTptAddr) GetDebugVals() directive.DebugValues {
	vals := directive.NewDebugValues()
	vals["peer-id"] = []string{d.DialTptAddrTargetPeerId().String()}
	vals["tpt-addr"] = []string{d.DialTptAddrDialerOpts().GetAddress()}
	if src := d.DialTptAddrSourcePeerId(); len(src) != 0 {
		vals["from-peer-id"] = []string{d.DialTptAddrSourcePeerId().String()}
	}
	return vals
}

// _ is a type constraint
var _ DialTptAddr = ((*dialTptAddr)(nil))
