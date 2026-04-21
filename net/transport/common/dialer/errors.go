package dialer

import "errors"

var (
	// ErrNotTransportDialer is returned if a Transport does not implement TransportDialer.
	ErrNotTransportDialer = errors.New("transport does not implement a dialer")
	// ErrEmptyAddress is returned if the address field was empty.
	ErrEmptyAddress = errors.New("dialer opts address cannot be empty")
)
