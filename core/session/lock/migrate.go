package session_lock

import (
	"bytes"
	"context"
	"slices"

	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
)

// CopyCredential atomically preserves this Session's PIN protection, recovery
// envelope, and setup state. Auto-unlock is rewrapped under the destination
// volume key. An existing credential is retained only when it is the same key;
// a different or unprovable credential fails without changing either store.
func CopyCredential(ctx context.Context, source, destination object.ObjectStore, sourceID, destinationID string, key crypto.PrivKey, destinationKey [32]byte) error {
	// Read the credential records carried between the two stores.
	suffixes := [][]byte{SuffixPK, SuffixLocked, SuffixLockKey, SuffixLockParams, SuffixEnvelope, SuffixSetupDone}
	values := make(map[string][]byte, len(suffixes))
	if err := kvtx.RunTransaction(ctx, false, func(ctx context.Context) (kvtx.Tx, error) {
		return source.NewTransaction(ctx, false)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		clear(values)
		for _, suffix := range suffixes {
			data, found, err := tx.Get(ctx, MakeKey(sourceID, suffix))
			if err != nil {
				return err
			}
			if found {
				values[string(suffix)] = slices.Clone(data)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	pin := len(values[string(SuffixLockParams)]) != 0
	if pin {
		if len(values[string(SuffixLocked)]) == 0 || len(values[string(SuffixLockKey)]) == 0 {
			return errors.New("source Session has incomplete PIN protection")
		}
		config := &LockConfig{}
		if err := config.UnmarshalVT(values[string(SuffixLockParams)]); err != nil {
			return err
		}
		delete(values, string(SuffixPK))
	} else {
		if len(values[string(SuffixPK)]) == 0 {
			return errors.New("source Session credential is missing")
		}
		plain, err := keypem.MarshalPrivKeyPem(key)
		if err != nil {
			return err
		}
		defer scrub.Scrub(plain)
		encrypted, err := EncryptAutoUnlock(destinationKey, plain)
		if err != nil {
			return err
		}
		values[string(SuffixPK)] = encrypted
		delete(values, string(SuffixLocked))
		delete(values, string(SuffixLockKey))
		delete(values, string(SuffixLockParams))
	}
	return kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return destination.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		// Inspect identity and protection in the same transaction as admission.
		var existing bool
		var pinFields int
		for _, suffix := range suffixes[:4] {
			data, found, err := tx.Get(ctx, MakeKey(destinationID, suffix))
			if err != nil {
				return err
			}
			if !found {
				continue
			}
			existing = true
			if bytes.Equal(suffix, SuffixPK) {
				if pin {
					return errors.New("destination Session has different lock protection")
				}
				plain, err := DecryptAutoUnlock(destinationKey, data)
				if err != nil {
					return errors.New("destination Session credential conflicts with the moving Session")
				}
				stored, err := keypem.ParsePrivKeyPem(plain)
				scrub.Scrub(plain)
				if err != nil || !stored.Equals(key) {
					return errors.New("destination Session credential belongs to another key")
				}
			} else if !pin || !bytes.Equal(data, values[string(suffix)]) {
				return errors.New("destination Session has different PIN protection")
			} else {
				pinFields++
			}
		}
		if existing {
			if pin && pinFields != 3 {
				return errors.New("destination Session has incomplete PIN protection")
			}
			return nil
		}
		for _, suffix := range suffixes {
			if data, found := values[string(suffix)]; found {
				if err := tx.Set(ctx, MakeKey(destinationID, suffix), data); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
