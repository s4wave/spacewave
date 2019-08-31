package volume_store

import (
	"github.com/libp2p/go-libp2p-core/crypto"
)

// Store implements the volume state store.
type Store interface {
	// LoadPeerPriv attempts to load the volume private key.
	// May return nil if there is no key stored.
	LoadPeerPriv() (crypto.PrivKey, error)
	// StorePeerPriv overwrites the volume's stored private key.
	// Note: the store should transform the data to protect the key.
	StorePeerPriv(crypto.PrivKey) error
}
