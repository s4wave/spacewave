package provider_local

import (
	"bytes"
	"context"
	"slices"

	"github.com/pkg/errors"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
)

// ListStoredSessionIDs returns identities whose protected signing keys are
// already present in the account volume. It never mounts a Session, since
// mounting an unknown ID would generate a new key.
func (a *ProviderAccount) ListStoredSessionIDs(ctx context.Context) ([]string, error) {
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		a.t.p.b,
		false,
		SessionObjectStoreID(a.GetProviderID(), a.GetAccountID()),
		a.vol.GetID(),
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "mount session object store")
	}
	defer diRef.Release()

	var ids []string
	err = kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStoreHandle.GetObjectStore().NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			ids = nil
			seen := make(map[string]struct{})
			err := tx.ScanPrefixKeys(ctx, []byte{}, func(key []byte) error {
				var id []byte
				switch {
				case bytes.HasSuffix(key, session_lock.SuffixPK):
					id = bytes.TrimSuffix(key, session_lock.SuffixPK)
				case bytes.HasSuffix(key, session_lock.SuffixLockParams):
					id = bytes.TrimSuffix(key, session_lock.SuffixLockParams)
				default:
					return nil
				}
				if len(id) == 0 || bytes.ContainsRune(id, '/') {
					return nil
				}
				seen[string(id)] = struct{}{}
				return nil
			})
			if err != nil {
				return err
			}
			for id := range seen {
				ids = append(ids, id)
			}
			return nil
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "scan stored session keys")
	}
	slices.Sort(ids)
	return ids, nil
}
