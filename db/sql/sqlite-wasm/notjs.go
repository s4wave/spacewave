//go:build !js

package sqlite_wasm

import (
	"github.com/pkg/errors"
)

var errNotAvailable = errors.New("sqlite-wasm: only available on js/wasm")

// SetClient is a no-op on non-js platforms.
func SetClient(_ any) {}

// DeleteDatabase is not available on non-js platforms.
func DeleteDatabase(_ string) error {
	return errNotAvailable
}
