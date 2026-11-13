package s4wave_secret

import "testing"

// DeleteSecretWithHook exposes the delete with a hook after each held account,
// for tests that interrupt or race the window before the next copy or the name.
var DeleteSecretWithHook = deleteSecret

// SetDeletePayloadOff selects DeletePayloadEnv's old behavior for one test.
func SetDeletePayloadOff(t *testing.T, off bool) {
	prev := deletePayloadOff
	deletePayloadOff = off
	t.Cleanup(func() { deletePayloadOff = prev })
}
