package session

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// Session is a handle referring to an active session.
type Session interface {
	// GetBus returns the bus used by the session.
	GetBus() bus.Bus

	// GetSessionRef returns the ref to the session.
	GetSessionRef() *SessionRef

	// GetPeerId returns the peer id used by the session.
	GetPeerId() peer.ID

	// GetPrivKey returns the session private key.
	// Returns nil if the session is locked.
	GetPrivKey() crypto.PrivKey

	// GetProviderAccount returns the handle to the session provider account.
	GetProviderAccount() provider.ProviderAccount

	// AccessStateAtomStore gets or creates a shared session state atom store.
	AccessStateAtomStore(ctx context.Context, storeID string) (resource_state.StateAtomStore, error)

	// SnapshotStateAtomStoreIDs returns the known session state atom store ids.
	SnapshotStateAtomStoreIDs(ctx context.Context) ([]string, error)

	// WatchStateAtomStoreIDs watches the known session state atom store ids.
	WatchStateAtomStoreIDs(ctx context.Context, cb func(storeIDs []string) error) error

	// GetLockState returns the current lock mode and whether the session is locked.
	GetLockState(ctx context.Context) (SessionLockMode, bool, error)

	// WatchLockState calls the callback with the current lock state and on changes.
	// Blocks until ctx is canceled or an error occurs.
	WatchLockState(ctx context.Context, cb func(mode SessionLockMode, locked bool)) error

	// UnlockSession unlocks a PIN-locked session with the given PIN.
	UnlockSession(ctx context.Context, pin []byte) error

	// SetLockMode changes the session lock mode.
	// Pin is required when mode is PIN_ENCRYPTED.
	SetLockMode(ctx context.Context, mode SessionLockMode, pin []byte) error

	// LockSession locks a running session, scrubbing the privkey and
	// requiring PIN re-entry. Only works when PIN mode is configured.
	LockSession(ctx context.Context) error
}
