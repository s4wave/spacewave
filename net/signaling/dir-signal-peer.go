package signaling

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/peer"
)

// SignalPeer is a directive to establish a signaling channel with a remote peer.
type SignalPeer interface {
	// Directive indicates SignalPeer is a directive.
	directive.Directive

	// SignalingID returns the identifier of the signaling channel to use.
	// Can be empty to use any channel.
	SignalingID() string
	// SignalLocalPeerID returns the local peer ID to signal from.
	// Can be empty.
	SignalLocalPeerID() peer.ID
	// SignalRemotePeerID returns the remote peer ID to signal to.
	// Cannot be empty.
	SignalRemotePeerID() peer.ID
}

// SignalPeerValue is the value of the SignalPeer directive.
type SignalPeerValue = SignalPeerSession

// signalPeer implements SignalPeer
type signalPeer struct {
	signalingID  string
	localPeerID  peer.ID
	remotePeerID peer.ID
}

// NewSignalPeer constructs a new SignalPeer directive.
func NewSignalPeer(signalingID string, localPeerID, remotePeerID peer.ID) SignalPeer {
	return &signalPeer{
		signalingID:  signalingID,
		localPeerID:  localPeerID,
		remotePeerID: remotePeerID,
	}
}

// ExSignalPeer opens a SignalPeerSession with the remote peer ID.
//
// signalingID and localPeerID are optional
// returns a release function
// if returnIfIdle: returns nil, nil, nil if not found (idle directive)
func ExSignalPeer(
	ctx context.Context,
	b bus.Bus,
	signalingID string,
	localPeerID, remotePeerID peer.ID,
	returnIfIdle bool,
) (SignalPeerValue, func(), error) {
	estl, _, ref, err := bus.ExecWaitValue[SignalPeerValue](
		ctx,
		b,
		NewSignalPeer(
			signalingID,
			localPeerID, remotePeerID,
		),
		bus.ReturnIfIdle(returnIfIdle),
		nil,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	if estl == nil {
		ref.Release()
		return nil, nil, nil
	}

	return estl, ref.Release, nil
}

// SignalingID returns the identifier of the signaling channel to use.
func (d *signalPeer) SignalingID() string {
	return d.signalingID
}

// SignalLocalPeerID returns the local peer ID to signal from.
// Can be empty.
func (d *signalPeer) SignalLocalPeerID() peer.ID {
	return d.localPeerID
}

// SignalRemotePeerID returns the remote peer ID to signal to.
// Cannot be empty.
func (d *signalPeer) SignalRemotePeerID() peer.ID {
	return d.remotePeerID
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *signalPeer) Validate() error {
	if d.remotePeerID == "" {
		return peer.ErrEmptyPeerID
	}
	if _, err := d.remotePeerID.ExtractPublicKey(); err != nil {
		return err
	}
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *signalPeer) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *signalPeer) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(SignalPeer)
	if !ok {
		return false
	}

	return d.localPeerID.String() == od.SignalLocalPeerID().String() &&
		d.remotePeerID.String() == od.SignalRemotePeerID().String() &&
		d.signalingID == od.SignalingID()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *signalPeer) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *signalPeer) GetName() string {
	return "SignalPeer"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *signalPeer) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{
		"remote-peer": []string{d.remotePeerID.String()},
	}
	if d.localPeerID != "" {
		vals["local-peer"] = []string{d.localPeerID.String()}
	}
	if d.signalingID != "" {
		vals["signaling-id"] = []string{d.signalingID}
	}
	return vals
}

// _ is a type assertion
var _ SignalPeer = ((*signalPeer)(nil))
