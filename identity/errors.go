package identity

import "errors"

// ErrUnableDerivePrivKey is returned if no matching private keys could be
// derived.
var ErrUnableDerivePrivKey = errors.New("unable to derive any private key")
