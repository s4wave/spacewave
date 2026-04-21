package transport_quic

import "errors"

var (
	// ErrDialUnimplemented is returned if dialing the peer is unimplemented.
	ErrDialUnimplemented = errors.New("dial peer not implemented")
	// ErrRemoteUnspecified is returned if the remote addr is unspecified.
	ErrRemoteUnspecified = errors.New("peer id and/or remote addr must be specified")
)
