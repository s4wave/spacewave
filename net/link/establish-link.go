package link

import (
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// holdOpenDur is the default hold open duration
var holdOpenDur = time.Second * 10

// EstablishLinkWithPeer is a directive to establish a link with a peer.
//
// Value: Link
type EstablishLinkWithPeer interface {
	// Directive indicates EstablishLinkWithPeer is a directive.
	directive.Directive

	// EstablishLinkSourcePeerId returns the source peer ID.
	// Can be empty to allow any.
	EstablishLinkSourcePeerId() peer.ID
	// EstablishLinkTargetPeerId returns the target peer ID.
	// Cannot be empty.
	EstablishLinkTargetPeerId() peer.ID
}

// EstablishLinkWithPeerValue is the type emitted when resolving EstablishLinkWithPeer.
type EstablishLinkWithPeerValue = MountedLink

// establishLinkWithPeer implements EstablishLinkWithPeer with a peer ID constraint.
type establishLinkWithPeer struct {
	src, dest peer.ID
}

// NewEstablishLinkWithPeer constructs a new EstablishLinkWithPeer directive.
func NewEstablishLinkWithPeer(srcPeer, destPeer peer.ID) EstablishLinkWithPeer {
	return &establishLinkWithPeer{src: srcPeer, dest: destPeer}
}

// EstablishLinkTargetPeerId returns the target peer ID.
func (d *establishLinkWithPeer) EstablishLinkTargetPeerId() peer.ID {
	return d.dest
}

// EstablishLinkSourcePeerId returns the src peer ID.
func (d *establishLinkWithPeer) EstablishLinkSourcePeerId() peer.ID {
	return d.src
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *establishLinkWithPeer) Validate() error {
	if len(d.dest) == 0 {
		return errors.Wrap(peer.ErrEmptyPeerID, "destination")
	}

	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *establishLinkWithPeer) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur: holdOpenDur,
	}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *establishLinkWithPeer) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(EstablishLinkWithPeer)
	if !ok {
		return false
	}

	return d.EstablishLinkTargetPeerId() == od.EstablishLinkTargetPeerId() &&
		d.EstablishLinkSourcePeerId() == od.EstablishLinkSourcePeerId()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *establishLinkWithPeer) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *establishLinkWithPeer) GetName() string {
	return "EstablishLinkWithPeer"
}

// GetDebugVals returns the directive arguments as k/v pairs.
// This is not necessarily unique, and is primarily intended for display.
func (d *establishLinkWithPeer) GetDebugVals() directive.DebugValues {
	vals := directive.NewDebugValues()
	vals["dst-peer"] = []string{d.EstablishLinkTargetPeerId().String()}
	if src := d.EstablishLinkSourcePeerId(); len(src) != 0 {
		vals["src-peer"] = []string{d.EstablishLinkSourcePeerId().String()}
	}
	return vals
}

// _ is a type constraint
var _ EstablishLinkWithPeer = ((*establishLinkWithPeer)(nil))
