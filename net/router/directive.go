package router

import (
	"errors"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
)

// DiscoverRoutes is a directive to discover routes to a peer.
type DiscoverRoutes interface {
	// Directive indicates DiscoverRoutes is a directive.
	directive.Directive

	// DiscoverRoutesLocalPeerID returns the local peer ID we are
	// routing from.
	DiscoverRoutesLocalPeerID() peer.ID
	// DiscoverRoutesRemotePeerID returns the remote peer ID we are
	// attempting to route to.
	DiscoverRoutesRemotePeerID() peer.ID
}

// DiscoverRoutesWithPeerIDs implements DiscoverRoutes with a
// protocol ID and peer ID.
type DiscoverRoutesWithPeerIDs struct {
	protocolID   protocol.ID
	localPeerID  peer.ID
	remotePeerID peer.ID
}

// NewDiscoverRoutesWithPeerIDs constructs a new DiscoverRoutesWithPeerIDs directive.
func NewDiscoverRoutesWithPeerIDs(
	protocolID protocol.ID,
	localPeerID, remotePeerID peer.ID,
) *DiscoverRoutesWithPeerIDs {
	return &DiscoverRoutesWithPeerIDs{
		protocolID:   protocolID,
		localPeerID:  localPeerID,
		remotePeerID: remotePeerID,
	}
}

// DiscoverRoutesLocalPeerID returns the local peer ID we are
// requesting a handler for. Cannot be empty.
func (d *DiscoverRoutesWithPeerIDs) DiscoverRoutesLocalPeerID() peer.ID {
	return d.localPeerID
}

// DiscoverRoutesRemotePeerID returns the remote peer ID we are
// requesting a handler for. Cannot be empty.
func (d *DiscoverRoutesWithPeerIDs) DiscoverRoutesRemotePeerID() peer.ID {
	return d.remotePeerID
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *DiscoverRoutesWithPeerIDs) Validate() error {
	if len(d.localPeerID) == 0 {
		return errors.New("local peer id required")
	}
	if len(d.remotePeerID) == 0 {
		return errors.New("remote peer id required")
	}
	if len(d.protocolID) == 0 {
		return errors.New("protocol id required")
	}

	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *DiscoverRoutesWithPeerIDs) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		MaxValueCount:   1,
		MaxValueHardCap: true,
	}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *DiscoverRoutesWithPeerIDs) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(DiscoverRoutes)
	if !ok {
		return false
	}

	return d.localPeerID == od.DiscoverRoutesLocalPeerID()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *DiscoverRoutesWithPeerIDs) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *DiscoverRoutesWithPeerIDs) GetName() string {
	return "DiscoverRoutesWithPeerIDs"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *DiscoverRoutesWithPeerIDs) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if pid := d.DiscoverRoutesLocalPeerID(); pid != peer.ID("") {
		vals["src-peer"] = []string{pid.String()}
	}
	if pid := d.DiscoverRoutesRemotePeerID(); pid != peer.ID("") {
		vals["dst-peer"] = []string{pid.String()}
	}
	return vals
}

// _ is a type constraint
var _ DiscoverRoutes = ((*DiscoverRoutesWithPeerIDs)(nil))
