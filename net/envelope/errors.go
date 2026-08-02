package envelope

import "github.com/pkg/errors"

// ErrNoGrants is returned when the envelope has no grants.
var ErrNoGrants = errors.New("envelope has no grants")

// ErrNoKeypairs is returned when the envelope has no keypairs.
var ErrNoKeypairs = errors.New("envelope has no keypairs")

// ErrInvalidThreshold is returned when the threshold configuration is invalid.
var ErrInvalidThreshold = errors.New("invalid threshold configuration")

// ErrContextMismatch is returned when the envelope context does not match the expected context.
var ErrContextMismatch = errors.New("envelope context does not match expected context")

// ErrInvalidKeypairIndex is returned when a keypair index is out of range.
var ErrInvalidKeypairIndex = errors.New("keypair index out of range")

// ErrEmptyPayload is returned when the payload is empty.
var ErrEmptyPayload = errors.New("payload is empty")

// ErrDecryptionFailed is returned when envelope decryption fails.
var ErrDecryptionFailed = errors.New("envelope decryption failed")
