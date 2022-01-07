package blockenc

import "errors"

// ErrShortKey is returned if the key was too short.
var ErrShortKey = errors.New("key too short")

// ErrShortMsg is returned if the encrypted message was too short.
var ErrShortMsg = errors.New("message too short")

// ErrDecryptFail is returned if the decrypt operation failed.
var ErrDecryptFail = errors.New("failed to decrypt message")
