package identity

import "errors"

var (
	// ErrUnableDerivePrivKey is returned if we could not derive any matching private keys.
	ErrUnableDerivePrivKey = errors.New("unable to derive any private key")
)
