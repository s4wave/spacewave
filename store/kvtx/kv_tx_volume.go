package store_kvtx

import (
	"github.com/aperturerobotics/bifrost/keypem"
	"github.com/libp2p/go-libp2p-crypto"
	"time"
)

// LoadPeerPriv attempts to load the peer private key from the volume.
func (k *KVTx) LoadPeerPriv() (crypto.PrivKey, error) {
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
func (k *KVTx) StorePeerPriv(privKey crypto.PrivKey) error {
	dat, err := keypem.MarshalPrivKeyPem(privKey)
	if err != nil {
		return err
	}

	tx, err := k.store.NewTransaction(true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// TODO: pre-shared key encryption

	err = tx.Set(k.kvkey.GetPeerPrivKey(), dat, time.Duration(0))
	if err != nil {
		return err
	}

	return tx.Commit(k.ctx)
}
