package link

import (
	"errors"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
)

// HandleMountedStream is a directive to return a mounted stream handler for a
// protocol ID.
//
// Value is of type link.MountedStreamHandler.
type HandleMountedStream interface {
	// Directive indicates HandleMountedStream is a directive.
	directive.Directive

	// HandleMountedStreamProtocolID returns the protocol ID we are requesting a
	// handler for. Cannot be empty.
	HandleMountedStreamProtocolID() protocol.ID
	// HandleMountedStreamLocalPeerID returns the local peer ID we are
	// requesting a handler for. Cannot be empty.
	HandleMountedStreamLocalPeerID() peer.ID
	// HandleMountedStreamRemotePeerID returns the remote peer ID we are
	// requesting a handler for. Cannot be empty.
	HandleMountedStreamRemotePeerID() peer.ID
}

// HandleMountedStreamValue is the value type for HandleMountedStream.
type HandleMountedStreamValue = MountedStreamHandler

// handleMountedStream implements HandleMountedStream with a protocol ID and
// peer ID.
type handleMountedStream struct {
	protocolID   protocol.ID
	localPeerID  peer.ID
	remotePeerID peer.ID
}

// NewHandleMountedStream constructs a new HandleMountedStream directive.
func NewHandleMountedStream(
	protocolID protocol.ID,
	localPeerID, remotePeerID peer.ID,
) HandleMountedStream {
	return &handleMountedStream{
		protocolID:   protocolID,
		localPeerID:  localPeerID,
		remotePeerID: remotePeerID,
	}
}

// HandleMountedStreamProtocolID returns the protocol ID we are requesting a
// handler for. Cannot be empty.
func (d *handleMountedStream) HandleMountedStreamProtocolID() protocol.ID {
	return d.protocolID
}

// HandleMountedStreamLocalPeerID returns the local peer ID we are
// requesting a handler for. Cannot be empty.
func (d *handleMountedStream) HandleMountedStreamLocalPeerID() peer.ID {
	return d.localPeerID
}

// HandleMountedStreamRemotePeerID returns the remote peer ID we are
// requesting a handler for. Cannot be empty.
func (d *handleMountedStream) HandleMountedStreamRemotePeerID() peer.ID {
	return d.remotePeerID
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *handleMountedStream) Validate() error {
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
func (d *handleMountedStream) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		MaxValueCount:   1,
		MaxValueHardCap: true,

		UnrefDisposeEmptyImmediate: true,
		UnrefDisposeDur:            time.Second,
	}
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *handleMountedStream) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(HandleMountedStream)
	if !ok {
		return false
	}

	return d.protocolID == od.HandleMountedStreamProtocolID() &&
		d.localPeerID == od.HandleMountedStreamLocalPeerID() &&
		d.remotePeerID == od.HandleMountedStreamRemotePeerID()
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *handleMountedStream) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *handleMountedStream) GetName() string {
	return "HandleMountedStream"
}

// GetDebugVals returns the directive arguments as k/v pairs.
// This is not necessarily unique, and is primarily intended for display.
func (d *handleMountedStream) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["protocol-id"] = []string{string(d.HandleMountedStreamProtocolID())}
	vals["local-peer"] = []string{d.HandleMountedStreamLocalPeerID().String()}
	vals["remote-peer"] = []string{d.HandleMountedStreamRemotePeerID().String()}
	return vals
}

// _ is a type constraint
var _ HandleMountedStream = ((*handleMountedStream)(nil))
