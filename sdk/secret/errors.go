package s4wave_secret

import "errors"

var (
	// ErrMissingSecretRef is returned when a Secret has no nested SharedObject ref.
	ErrMissingSecretRef = errors.New("secret: missing nested shared object ref")
	// ErrPayloadAccessDenied is returned when the nested SharedObject payload cannot be decrypted.
	ErrPayloadAccessDenied = errors.New("secret: payload access denied")
)
