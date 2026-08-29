package provider_local

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
)

// spacelinkNonceKeyPrefix namespaces consumed SpaceLink ticket nonces inside
// the account session object store.
var spacelinkNonceKeyPrefix = []byte("spacelink/nonce/")

// spacelinkNonceKey returns the object store key for one ticket nonce.
func spacelinkNonceKey(nonce []byte) []byte {
	return append(spacelinkNonceKeyPrefix, []byte(hex.EncodeToString(nonce))...)
}

// ConsumeSpaceLinkNonce atomically records a consumed SpaceLink ticket nonce
// for this account. A replayed nonce is rejected before any state mutation.
func (a *ProviderAccount) ConsumeSpaceLinkNonce(
	ctx context.Context,
	agentPeerID string,
	nonce []byte,
	payload []byte,
	expiresAt time.Time,
) error {
	if len(nonce) == 0 {
		return errors.New("spacelink nonce is required")
	}

	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		a.t.p.b,
		false,
		SessionObjectStoreID(a.GetProviderID(), a.GetAccountID()),
		a.vol.GetID(),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "mount session object store")
	}
	defer diRef.Release()

	key := spacelinkNonceKey(nonce)
	value := fmt.Sprintf("%s\x00%s", agentPeerID, hex.EncodeToString(payload))
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStoreHandle.GetObjectStore().NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			found, err := tx.Exists(ctx, key)
			if err != nil {
				return errors.Wrap(err, "check spacelink nonce")
			}
			if found {
				return errors.New("spacelink nonce was already consumed")
			}
			if err := tx.Set(ctx, key, []byte(value)); err != nil {
				return errors.Wrap(err, "record spacelink nonce")
			}
			return nil
		},
	)
	return errors.Wrap(err, "consume spacelink nonce")
}
