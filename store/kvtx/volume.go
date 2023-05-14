package store_kvtx

import (
	"context"

	"github.com/aperturerobotics/bifrost/keypem"
	"github.com/libp2p/go-libp2p/core/crypto"
)

// LoadPeerPriv attempts to load the peer private key from the volume.
func (k *KVTx) LoadPeerPriv(ctx context.Context) (crypto.PrivKey, error) {
	tx, err := k.store.NewTransaction(false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	data, found, err := tx.Get(k.kvkey.GetPeerPrivKey())
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
	dat, err := keypem.MarshalPrivKeyPem(privKey)
	if err != nil {
		return err
	}

	tx, err := k.store.NewTransaction(true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	err = tx.Set(k.kvkey.GetPeerPrivKey(), dat)
	if err != nil {
		return err
	}

	return tx.Commit(k.ctx)
}
