package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
)

// sharedObjectRecoveryDecoder decrypts recovery envelopes using unlocked entity keys.
type sharedObjectRecoveryDecoder struct {
	entityPrivKeys []bifrost_crypto.PrivKey
}

// GetSelfEntityID returns the stable entity ID for the mounted account.
func (a *ProviderAccount) GetSelfEntityID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if a.accountID == "" {
		return "", errors.New("account id is required")
	}
	return a.accountID, nil
}

// ReadSharedObjectRecoveryEnvelope reads the recovery envelope for an SO.
func (a *ProviderAccount) ReadSharedObjectRecoveryEnvelope(ctx context.Context, ref *sobject.SharedObjectRef) (*sobject.SOEntityRecoveryEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, errors.New("shared object ref is required")
	}
	cli, _, _, err := a.getReadySessionClient(ctx)
	if err != nil {
		return nil, err
	}
	return cli.GetSORecoveryEnvelope(
		ctx,
		ref.GetProviderResourceRef().GetId(),
	)
}

// GetSharedObjectRecoveryDecoder returns a decoder backed by unlocked entity keys.
func (a *ProviderAccount) GetSharedObjectRecoveryDecoder(ctx context.Context) (sobject.SharedObjectRecoveryDecoder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store := a.getEntityKeyStore()
	if store == nil {
		return nil, sobject.ErrSharedObjectRecoveryCredentialRequired
	}
	keys := store.GetUnlockedKeys()
	if len(keys) == 0 {
		return nil, sobject.ErrSharedObjectRecoveryCredentialRequired
	}
	return &sharedObjectRecoveryDecoder{
		entityPrivKeys: keys,
	}, nil
}

// DecryptSharedObjectRecoveryEnvelope decrypts the recovery envelope.
func (d *sharedObjectRecoveryDecoder) DecryptSharedObjectRecoveryEnvelope(ctx context.Context, env *sobject.SOEntityRecoveryEnvelope) (*sobject.SOEntityRecoveryMaterial, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return sobject.UnlockSOEntityRecoveryEnvelope(d.entityPrivKeys, env)
}

// _ is a type assertion
var _ sobject.SharedObjectRecoveryProvider = ((*ProviderAccount)(nil))
