package identity

import "errors"

// ErrUnableDerivePrivKey is returned if we could not derive any matching private keys.
var ErrUnableDerivePrivKey = errors.New("unable to derive any private key")
