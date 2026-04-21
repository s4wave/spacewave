package transport

import "github.com/aperturerobotics/controllerbus/config"

// Config contains common parameters all transport configs must have.
type Config interface {
	// Config indicates this is a controllerbus config.
	config.Config
	// Debuggable indicates this config is debuggable.
	config.Debuggable
	// GetTransportPeerId returns the node peer ID constraint.
	GetTransportPeerId() string
	// SetTransportPeerId sets the node peer ID field.
	SetTransportPeerId(peerID string)
}
