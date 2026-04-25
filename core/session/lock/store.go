package session_lock

import (
	"context"

	"github.com/s4wave/spacewave/db/object"
)

// SessionLockMode identifies how a session private key is protected at rest.
type SessionLockMode int32

const (
	// SessionLockMode_AUTO_UNLOCK is encrypted with volume-derived key.
	SessionLockMode_AUTO_UNLOCK SessionLockMode = 0
	// SessionLockMode_PIN_ENCRYPTED is encrypted with PIN-derived key.
	SessionLockMode_PIN_ENCRYPTED SessionLockMode = 1
)

// ObjectStore key suffixes.
var (
	SuffixPK         = []byte("/pk")
	SuffixEnvelope   = []byte("/env")
	SuffixLocked     = []byte("/locked")
	SuffixLockKey    = []byte("/lock-key")
	SuffixLockParams = []byte("/lock-params")
	SuffixSetupDone  = []byte("/setup-done")
)

// MakeKey constructs an ObjectStore key from a session ID and suffix.
func MakeKey(sessionID string, suffix []byte) []byte {
	return append([]byte(sessionID), suffix...)
}

// ReadLockMode checks ObjectStore to determine lock mode.
// Returns PIN_ENCRYPTED if lock-params exists, AUTO_UNLOCK otherwise.
func ReadLockMode(ctx context.Context, objStore object.ObjectStore, sessionID string) (SessionLockMode, error) {
	otx, err := objStore.NewTransaction(ctx, false)
	if err != nil {
		return 0, err
	}
	defer otx.Discard()

	_, found, err := otx.Get(ctx, MakeKey(sessionID, SuffixLockParams))
	if err != nil {
		return 0, err
	}
	if found {
		return SessionLockMode_PIN_ENCRYPTED, nil
	}
	return SessionLockMode_AUTO_UNLOCK, nil
}

// ReadAutoUnlockKey reads the encrypted privkey for auto-unlock mode.
func ReadAutoUnlockKey(ctx context.Context, objStore object.ObjectStore, sessionID string) ([]byte, bool, error) {
	otx, err := objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, false, err
	}
	defer otx.Discard()
	return otx.Get(ctx, MakeKey(sessionID, SuffixPK))
}

// ReadPINLockFiles reads the encrypted privkey, encrypted symkey, and lock config.
func ReadPINLockFiles(ctx context.Context, objStore object.ObjectStore, sessionID string) (encPriv, encSymKey []byte, config *LockConfig, err error) {
	otx, err := objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, nil, err
	}
	defer otx.Discard()

	encPriv, found, err := otx.Get(ctx, MakeKey(sessionID, SuffixLocked))
	if err != nil {
		return nil, nil, nil, err
	}
	if !found {
		return nil, nil, nil, nil
	}

	encSymKey, found, err = otx.Get(ctx, MakeKey(sessionID, SuffixLockKey))
	if err != nil {
		return nil, nil, nil, err
	}
	if !found {
		return nil, nil, nil, nil
	}

	configData, found, err := otx.Get(ctx, MakeKey(sessionID, SuffixLockParams))
	if err != nil {
		return nil, nil, nil, err
	}
	if !found {
		return nil, nil, nil, nil
	}

	config = &LockConfig{}
	if err := config.UnmarshalVT(configData); err != nil {
		return nil, nil, nil, err
	}

	return encPriv, encSymKey, config, nil
}

// WriteAutoUnlock writes encrypted privkey for auto-unlock mode and deletes
// any PIN lock files.
func WriteAutoUnlock(ctx context.Context, objStore object.ObjectStore, sessionID string, encPriv []byte) error {
	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer otx.Discard()

	if err := otx.Set(ctx, MakeKey(sessionID, SuffixPK), encPriv); err != nil {
		return err
	}
	// Delete PIN lock files (mode switch).
	_ = otx.Delete(ctx, MakeKey(sessionID, SuffixLocked))
	_ = otx.Delete(ctx, MakeKey(sessionID, SuffixLockKey))
	_ = otx.Delete(ctx, MakeKey(sessionID, SuffixLockParams))
	return otx.Commit(ctx)
}

// WritePINLock writes PIN-encrypted lock files and deletes auto-unlock /pk file.
func WritePINLock(ctx context.Context, objStore object.ObjectStore, sessionID string, encPriv, encSymKey []byte, config *LockConfig) error {
	configBytes, err := config.MarshalVT()
	if err != nil {
		return err
	}

	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer otx.Discard()

	if err := otx.Set(ctx, MakeKey(sessionID, SuffixLocked), encPriv); err != nil {
		return err
	}
	if err := otx.Set(ctx, MakeKey(sessionID, SuffixLockKey), encSymKey); err != nil {
		return err
	}
	if err := otx.Set(ctx, MakeKey(sessionID, SuffixLockParams), configBytes); err != nil {
		return err
	}
	// Delete auto-unlock file.
	_ = otx.Delete(ctx, MakeKey(sessionID, SuffixPK))
	return otx.Commit(ctx)
}

// WriteEnvelope writes the Shamir envelope bytes to ObjectStore.
func WriteEnvelope(ctx context.Context, objStore object.ObjectStore, sessionID string, envData []byte) error {
	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer otx.Discard()
	if err := otx.Set(ctx, MakeKey(sessionID, SuffixEnvelope), envData); err != nil {
		return err
	}
	return otx.Commit(ctx)
}
