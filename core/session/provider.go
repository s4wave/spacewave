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

	// ResetPINSession resets a PIN-locked session according to the provider's
	// recovery model. Local providers require a credential and preserve the
	// session identity through envelope recovery; cloud providers may clear the
	// lock and allow a replacement key on the next mount.
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
