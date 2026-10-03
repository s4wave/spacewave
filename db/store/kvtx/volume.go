package store_kvtx

import (
	"context"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
)

// LoadPeerPriv attempts to load the peer private key from the volume.
func (k *KVTx) LoadPeerPriv(ctx context.Context) (crypto.PrivKey, error) {
	// Open a read transaction for the volume's peer key.
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Read the stored peer key, leaving an absent key unset.
	data, found, err := tx.Get(ctx, k.kvkey.GetPeerPrivKey())
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || !found {
		return nil, nil
	}

	return keypem.ParsePrivKeyPem(data)
}

// StorePeerPriv overwrites the volume's stored private key.
func (k *KVTx) StorePeerPriv(ctx context.Context, privKey crypto.PrivKey) error {
	// Encode the peer private key for storage in the volume.
	dat, err := keypem.MarshalPrivKeyPem(privKey)
	if err != nil {
		return err
	}

	// Open a write transaction for the volume's peer key.
	tx, err := k.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Replace the volume's stored peer key with the encoded key.
	err = tx.Set(ctx, k.kvkey.GetPeerPrivKey(), dat)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
