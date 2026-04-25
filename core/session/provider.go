package session

import (
	"context"

	provider "github.com/s4wave/spacewave/core/provider"
)

// SessionProvider implements ProviderFeature_SESSION.
type SessionProvider interface {
	provider.ProviderAccountFeature

	// MountSession attempts to mount a Session returning the handle and a release function.
	//
	// note: use the MountSession directive to call this.
	// usually called by the provider controller
	MountSession(ctx context.Context, ref *SessionRef, released func()) (Session, func(), error)

	// UnlockPINSession unlocks a PIN-locked session before it is mounted.
	// Reads the PIN lock files, decrypts the session key with the PIN,
	// and unblocks the session tracker so mounting can proceed.
	UnlockPINSession(ctx context.Context, ref *SessionRef, pin []byte) error

	// ResetPINSession resets a PIN-locked session. If a credential is
	// provided, attempts envelope recovery to preserve the session
	// identity. Falls back to regeneration (new key) if recovery fails
	// or credential is nil.
	ResetPINSession(ctx context.Context, ref *SessionRef, cred *EntityCredential) error
}

// GetSessionProviderAccountFeature returns the SessionProvider for a ProviderAccount.
func GetSessionProviderAccountFeature(ctx context.Context, provAcc provider.ProviderAccount) (SessionProvider, error) {
	return provider.GetProviderAccountFeature[SessionProvider](
		ctx,
		provAcc,
		provider.ProviderFeature_ProviderFeature_SESSION,
	)
}
