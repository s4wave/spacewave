package volume_store

import (
	"context"

	"github.com/aperturerobotics/bifrost/crypto"
)

// Store implements the volume state store.
type Store interface {
	// LoadPeerPriv attempts to load the volume private key.
	// May return nil if there is no key stored.
	LoadPeerPriv(ctx context.Context) (crypto.PrivKey, error)
	// StorePeerPriv overwrites the volume's stored private key.
	// Note: the store should transform the data to protect the key.
	StorePeerPriv(ctx context.Context, privKey crypto.PrivKey) error
}
