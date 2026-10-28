package s4wave_secret

import (
	"errors"
	"fmt"
)

var (
	// ErrMissingSecretRef is returned when a Secret has no nested SharedObject ref.
	ErrMissingSecretRef = errors.New("secret: missing nested shared object ref")
	// ErrPayloadAccessDenied is returned when the nested SharedObject payload cannot be decrypted.
	ErrPayloadAccessDenied = errors.New("secret: payload access denied")
	// ErrSecretKindMismatch is returned when a Secret is not the requested kind.
	ErrSecretKindMismatch = errors.New("secret: kind mismatch")
	// ErrReadChallengeNotFound is returned when a payload read challenge is unknown.
	ErrReadChallengeNotFound = errors.New("secret: read challenge not found")
	// ErrReadChallengeExpired is returned when a payload read challenge expired.
	ErrReadChallengeExpired = errors.New("secret: read challenge expired")
)

// accessDenied returns ErrPayloadAccessDenied with the reason access failed.
func accessDenied(reason string) error {
	return fmt.Errorf("%w: %s", ErrPayloadAccessDenied, reason)
}

// accessDeniedBy returns ErrPayloadAccessDenied with the failed step and its cause.
func accessDeniedBy(step string, cause error) error {
	return fmt.Errorf("%w: %s: %w", ErrPayloadAccessDenied, step, cause)
}
