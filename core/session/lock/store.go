package session_lock

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
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
	var mode = SessionLockMode_AUTO_UNLOCK
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			_, found, err := tx.Get(ctx, MakeKey(sessionID, SuffixLockParams))
			if err != nil {
				return err
			}
			if found {
				mode = SessionLockMode_PIN_ENCRYPTED
			}
			return nil
		},
	)
	return mode, err
}

// ReadAutoUnlockKey reads the encrypted privkey for auto-unlock mode.
func ReadAutoUnlockKey(ctx context.Context, objStore object.ObjectStore, sessionID string) ([]byte, bool, error) {
	var (
		data  []byte
		found bool
	)
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			var err error
			data, found, err = tx.Get(ctx, MakeKey(sessionID, SuffixPK))
			return err
		},
	)
	return data, found, err
}

// ReadPINLockFiles reads the encrypted privkey, encrypted symkey, and lock config.
func ReadPINLockFiles(ctx context.Context, objStore object.ObjectStore, sessionID string) (encPriv, encSymKey []byte, config *LockConfig, err error) {
	err = kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			var found bool
			encPriv, found, err = tx.Get(ctx, MakeKey(sessionID, SuffixLocked))
			if err != nil || !found {
				return err
			}

			encSymKey, found, err = tx.Get(ctx, MakeKey(sessionID, SuffixLockKey))
			if err != nil || !found {
				return err
			}

			var configData []byte
			configData, found, err = tx.Get(ctx, MakeKey(sessionID, SuffixLockParams))
			if err != nil || !found {
				return err
			}

			config = &LockConfig{}
			return config.UnmarshalVT(configData)
		},
	)
	return
}

// WriteAutoUnlock writes encrypted privkey for auto-unlock mode and deletes
// any PIN lock files.
func WriteAutoUnlock(ctx context.Context, objStore object.ObjectStore, sessionID string, encPriv []byte) error {
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			if err := tx.Set(ctx, MakeKey(sessionID, SuffixPK), encPriv); err != nil {
				return err
			}
			_ = tx.Delete(ctx, MakeKey(sessionID, SuffixLocked))
			_ = tx.Delete(ctx, MakeKey(sessionID, SuffixLockKey))
			_ = tx.Delete(ctx, MakeKey(sessionID, SuffixLockParams))
			return nil
		},
	)
}

// WritePINLock writes PIN-encrypted lock files and deletes auto-unlock /pk file.
func WritePINLock(ctx context.Context, objStore object.ObjectStore, sessionID string, encPriv, encSymKey []byte, config *LockConfig) error {
	configBytes, err := config.MarshalVT()
	if err != nil {
		return err
	}

	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			if err := tx.Set(ctx, MakeKey(sessionID, SuffixLocked), encPriv); err != nil {
				return err
			}
			if err := tx.Set(ctx, MakeKey(sessionID, SuffixLockKey), encSymKey); err != nil {
				return err
			}
			if err := tx.Set(ctx, MakeKey(sessionID, SuffixLockParams), configBytes); err != nil {
				return err
			}
			_ = tx.Delete(ctx, MakeKey(sessionID, SuffixPK))
			return nil
		},
	)
}

// WriteEnvelope writes the Shamir envelope bytes to ObjectStore.
func WriteEnvelope(ctx context.Context, objStore object.ObjectStore, sessionID string, envData []byte) error {
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, MakeKey(sessionID, SuffixEnvelope), envData)
		},
	)
}
