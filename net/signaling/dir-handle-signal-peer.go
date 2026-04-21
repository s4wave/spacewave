package signaling

import (
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
)

// HandleSignalPeer is a directive to handle an incoming signaling session.
//
// Does not expect any values to be resolved.
type HandleSignalPeer interface {
	// Directive indicates HandleSignalPeer is a directive.
	directive.Directive

	// SignalingID returns the identifier of the signaling channel.
	// Cannot be empty.
	HandleSignalingID() string
	// HandleSignalPeerSession returns the handle to the signaling session.
	// Cannot be empty.
	HandleSignalPeerSession() SignalPeerSession
}

// handleSignalPeer implements HandleSignalPeer
type handleSignalPeer struct {
	signalingID       string
	signalPeerSession SignalPeerSession
}

// NewHandleSignalPeer constructs a new HandleSignalPeer directive.
func NewHandleSignalPeer(signalingID string, signalPeerSession SignalPeerSession) HandleSignalPeer {
	return &handleSignalPeer{
		signalingID:       signalingID,
		signalPeerSession: signalPeerSession,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *handleSignalPeer) Validate() error {
	if d.signalingID == "" {
		return ErrEmptySignalingID
	}
	if d.signalPeerSession == nil {
		return errors.New("signal peer session cannot be nil")
	}
	if err := d.signalPeerSession.GetLocalPeerID().Validate(); err != nil {
		return errors.Wrap(err, "signal peer session: local peer")
	}
	if err := d.signalPeerSession.GetRemotePeerID().Validate(); err != nil {
		return errors.Wrap(err, "signal peer session: remote peer")
	}
	return nil
}

// SignalingID returns the identifier of the signaling channel.
// Cannot be empty.
func (d *handleSignalPeer) HandleSignalingID() string {
	return d.signalingID
}

// HandleSignalPeerSession returns the handle to the signaling session.
// Cannot be empty.
func (d *handleSignalPeer) HandleSignalPeerSession() SignalPeerSession {
	return d.signalPeerSession
}

// GetValueOptions returns options relating to value handling.
func (d *handleSignalPeer) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *handleSignalPeer) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(HandleSignalPeer)
	if !ok {
		return false
	}

	return d.signalingID == od.HandleSignalingID() &&
		d.signalPeerSession == od.HandleSignalPeerSession()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *handleSignalPeer) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *handleSignalPeer) GetName() string {
	return "HandleSignalPeer"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *handleSignalPeer) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{
		"signaling-id": []string{d.signalingID},
		"local-peer":   []string{d.signalPeerSession.GetLocalPeerID().String()},
		"remote-peer":  []string{d.signalPeerSession.GetRemotePeerID().String()},
	}
}

// _ is a type assertion
var (
	_ HandleSignalPeer     = ((*handleSignalPeer)(nil))
	_ directive.Debuggable = ((*handleSignalPeer)(nil))
)
